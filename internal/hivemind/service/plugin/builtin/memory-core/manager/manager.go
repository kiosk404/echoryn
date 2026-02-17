package manager

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/embedding"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
	meminternal "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/internal"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/internal/hybrid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/internal/search"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/store"
	"github.com/kiosk404/echoryn/pkg/logger"
	_ "github.com/mattn/go-sqlite3" // Register SQLite3 driver
)

// Manager is the core memory index manager.
// It maintains a SQLite database with indexed memory chunks and provides hybrid search.
type Manager struct {
	cfg      *entity.MemoryConfig
	provider embedding.Provider
	db       *sql.DB

	watcher *fsnotify.Watcher
	closeCh chan struct{}

	dirty   atomic.Bool
	syncing atomic.Bool
	closed  atomic.Bool

	ftsAvailable bool
	vecAvailable bool

	mu sync.RWMutex
}

// Global instance cache — matches OpenClaw's INDEX_CACHE pattern.
var (
	cacheMu sync.RWMutex
	cache   = make(map[string]*Manager)
)

// Get returns a cached Manager instance for the given config, or creates a new one.
// Matches OpenClaw's MemoryIndexManager.get() static factory.
func Get(ctx context.Context, cfg *entity.MemoryConfig) (*Manager, error) {
	key := cacheKey(cfg)

	cacheMu.RLock()
	if m, ok := cache[key]; ok {
		cacheMu.RUnlock()
		return m, nil
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// Double-check after acquiring write lock.
	if m, ok := cache[key]; ok {
		return m, nil
	}

	m, err := newManager(ctx, cfg)
	if err != nil {
		return nil, err
	}

	cache[key] = m
	return m, nil
}

// newManager creates and initializes a new Manager instance.
func newManager(ctx context.Context, cfg *entity.MemoryConfig) (*Manager, error) {
	logger.Info("[Memory] creating manager (workspace=%s, provider=%s, model=%s)",
		cfg.WorkspaceDir, cfg.Embedding.Provider, cfg.Embedding.Model)

	// Create embedding provider.
	providerResult, err := embedding.NewProvider(cfg.Embedding)
	if err != nil {
		return nil, fmt.Errorf("create embedding provider: %w", err)
	}

	if providerResult.FallbackFrom != "" {
		logger.Warn("[Memory] embedding provider fallback: %s -> %s (reason: %s)",
			providerResult.FallbackFrom, providerResult.Provider.ID(), providerResult.FallbackReason)
	}

	// Ensure database directory exists.
	dbPath := cfg.Store.Path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfg.WorkspaceDir, dbPath)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Open SQLite database.
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Initialize schema.
	ftsEnabled := cfg.Query.Hybrid.Enabled
	var vecConfig *store.VecSchemaConfig
	if cfg.Store.Vector.Enabled {
		vecConfig = &store.VecSchemaConfig{
			Enabled:       true,
			Dimensions:    embeddingDimensions(cfg.Embedding.Model),
			ExtensionPath: cfg.Store.Vector.ExtensionPath,
		}
	}
	schemaResult, err := store.EnsureSchema(db, ftsEnabled, vecConfig)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	if schemaResult.FTSError != "" {
		logger.Warn("[Memory] FTS5 unavailable: %s", schemaResult.FTSError)
	}
	if vecConfig != nil && schemaResult.VecError != "" {
		logger.Warn("[Memory] sqlite-vec unavailable: %s", schemaResult.VecError)
	}

	// Check if provider/model changed (needs full reindex).
	prevProvider, _ := store.GetMeta(db, store.MetaKeyProvider)
	prevModel, _ := store.GetMeta(db, store.MetaKeyModel)
	providerKey := embedding.ProviderKey(providerResult.Provider)

	needsFullReindex := prevProvider != providerResult.Provider.ID() || prevModel != providerResult.Provider.Model()
	if needsFullReindex && prevProvider != "" {
		logger.Info("[Memory] provider/model changed (%s/%s -> %s/%s), performing atomic rebuild...",
			prevProvider, prevModel, providerResult.Provider.ID(), providerResult.Provider.Model())

		// Atomic rebuild: wipe all chunks/FTS/vec data and rebuild.
		// This ensures no stale embeddings from the old model remain.
		if err := atomicClearIndex(db, schemaResult.FTSAvailable); err != nil {
			logger.Warn("[Memory] atomic rebuild cleanup failed: %v", err)
		}
	}

	// Update meta.
	store.SetMeta(db, store.MetaKeyProvider, providerResult.Provider.ID())
	store.SetMeta(db, store.MetaKeyModel, providerResult.Provider.Model())
	_ = providerKey

	m := &Manager{
		cfg:          cfg,
		provider:     providerResult.Provider,
		db:           db,
		closeCh:      make(chan struct{}),
		ftsAvailable: schemaResult.FTSAvailable,
		vecAvailable: schemaResult.VecAvailable,
	}

	// Mark dirty for initial sync if needed.
	if needsFullReindex {
		m.dirty.Store(true)
	}

	// Start file watcher.
	if cfg.Sync.Watch {
		if err := m.startWatcher(); err != nil {
			logger.Warn("[Memory] failed to start file watcher: %v", err)
		}
	}

	logger.Info("[Memory] manager created (fts=%v, vec=%v, reindex=%v)", m.ftsAvailable, m.vecAvailable, needsFullReindex)
	return m, nil
}

// Search performs a hybrid search (vector + keyword) on the memory index.
// Matches OpenClaw's MemoryIndexManager.search().
func (m *Manager) Search(ctx context.Context, query string, opts ...SearchOption) ([]entity.MemorySearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		return nil, fmt.Errorf("manager is closed")
	}

	cfg := m.cfg.Query
	for _, opt := range opts {
		opt(&cfg)
	}

	// Sync if dirty and onSearch is enabled.
	if m.cfg.Sync.OnSearch && m.dirty.Load() {
		m.mu.RUnlock()
		_ = m.Sync(ctx, SyncOpts{Reason: "search"})
		m.mu.RLock()
	}

	candidateLimit := int(float64(cfg.MaxResults) * cfg.Hybrid.CandidateMultiplier)
	if candidateLimit < cfg.MaxResults {
		candidateLimit = cfg.MaxResults
	}

	sourceFilter := m.cfg.Sources

	// Embed the query.
	queryVec, err := m.embedQueryWithTimeout(ctx, query)
	if err != nil {
		logger.Warn("[Memory] failed to embed query: %v", err)
	}

	// Vector search — use sqlite-vec KNN if available, otherwise brute-force.
	var vectorResults []hybrid.VectorResult
	if len(queryVec) > 0 {
		if m.vecAvailable {
			vectorResults, _ = search.SearchVectorVec(search.SearchVectorVecParams{
				DB:           m.db,
				QueryVec:     queryVec,
				Limit:        candidateLimit,
				SourceFilter: sourceFilter,
			})
		} else {
			vectorResults, _ = search.SearchVector(search.SearchVectorParams{
				DB:            m.db,
				ProviderModel: m.provider.Model(),
				QueryVec:      queryVec,
				Limit:         candidateLimit,
				SourceFilter:  sourceFilter,
			})
		}
	}

	// Keyword search (FTS).
	var keywordResults []hybrid.KeywordResult
	if m.ftsAvailable && cfg.Hybrid.Enabled {
		keywordResults, _ = search.SearchKeyword(search.SearchKeywordParams{
			DB:            m.db,
			ProviderModel: m.provider.Model(),
			Query:         query,
			Limit:         candidateLimit,
			SourceFilter:  sourceFilter,
		})
	}

	// Merge hybrid results.
	merged := hybrid.MergeResults(vectorResults, keywordResults, cfg.Hybrid.VectorWeight, cfg.Hybrid.TextWeight)

	// Filter by min score and limit.
	var filtered []entity.MemorySearchResult
	for _, r := range merged {
		if r.Score >= cfg.MinScore {
			filtered = append(filtered, r)
		}
		if len(filtered) >= cfg.MaxResults {
			break
		}
	}

	return filtered, nil
}

// Sync synchronizes the memory index with the filesystem.
// Matches OpenClaw's MemoryIndexManager.sync().
func (m *Manager) Sync(ctx context.Context, opts SyncOpts) error {
	// CAS to prevent concurrent syncs.
	if !m.syncing.CompareAndSwap(false, true) {
		return nil
	}
	defer m.syncing.Store(false)

	logger.Info("[Memory] starting sync (reason=%s)", opts.Reason)
	start := time.Now()

	if err := m.runSync(ctx, opts); err != nil {
		logger.Warn("[Memory] sync failed: %v", err)
		return err
	}

	m.dirty.Store(false)

	elapsed := time.Since(start)
	fileCount, _ := store.CountFiles(m.db)
	chunkCount, _ := store.CountChunks(m.db)
	logger.Info("[Memory] sync complete (reason=%s, files=%d, chunks=%d, elapsed=%s)",
		opts.Reason, fileCount, chunkCount, elapsed)

	return nil
}

// runSync executes the actual sync logic.
func (m *Manager) runSync(ctx context.Context, opts SyncOpts) error {
	// Sync memory files.
	files, err := meminternal.ListMemoryFiles(m.cfg.WorkspaceDir, m.cfg.ExtraPaths)
	if err != nil {
		return fmt.Errorf("list memory files: %w", err)
	}

	activePaths := make(map[string]struct{})
	for _, absPath := range files {
		entry, err := meminternal.BuildFileEntry(absPath, m.cfg.WorkspaceDir)
		if err != nil {
			logger.Warn("[Memory] failed to build file entry %s: %v", absPath, err)
			continue
		}
		activePaths[entry.Path] = struct{}{}

		// Check if file is unchanged.
		existingHash, found, _ := store.GetFileRecord(m.db, entry.Path, entity.MemorySourceMemory)
		if found && existingHash == entry.Hash && !opts.Force {
			continue
		}

		// Index this file.
		if err := m.indexFile(ctx, entry, entity.MemorySourceMemory); err != nil {
			logger.Warn("[Memory] failed to index %s: %v", entry.Path, err)
		}
	}

	// Clean stale entries.
	stalePaths, _ := store.GetStalePaths(m.db, entity.MemorySourceMemory)
	for _, stalePath := range stalePaths {
		if _, ok := activePaths[stalePath]; !ok {
			store.DeleteFileAndChunks(m.db, stalePath, entity.MemorySourceMemory, m.provider.Model(), m.ftsAvailable)
		}
	}

	// Prune embedding cache.
	if m.cfg.Cache.Enabled && m.cfg.Cache.MaxEntries > 0 {
		store.PruneEmbeddingCache(m.db, m.cfg.Cache.MaxEntries)
	}

	return nil
}

// indexFile indexes a single memory file: chunk → embed → store.
func (m *Manager) indexFile(ctx context.Context, entry *entity.MemoryFileEntry, source entity.MemorySource) error {
	content, err := os.ReadFile(entry.AbsPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Chunk the content.
	chunks := meminternal.ChunkMarkdown(string(content), m.cfg.Chunking)
	if len(chunks) == 0 {
		return nil
	}

	// Prepare texts for batch embedding.
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}

	// Check embedding cache.
	providerKey := embedding.ProviderKey(m.provider)
	hashes := make([]string, len(chunks))
	for i, chunk := range chunks {
		hashes[i] = chunk.Hash
	}

	cachedEmbeddings, _ := store.LoadEmbeddingCache(m.db, m.provider.ID(), m.provider.Model(), providerKey, hashes)

	// Find uncached texts.
	var uncachedIndices []int
	var uncachedTexts []string
	for i, chunk := range chunks {
		if _, ok := cachedEmbeddings[chunk.Hash]; !ok {
			uncachedIndices = append(uncachedIndices, i)
			uncachedTexts = append(uncachedTexts, texts[i])
		}
	}

	// Embed uncached texts.
	var newEmbeddings [][]float32
	if len(uncachedTexts) > 0 {
		var embedErr error
		newEmbeddings, embedErr = m.provider.EmbedBatch(ctx, uncachedTexts)
		if embedErr != nil {
			return fmt.Errorf("embed batch: %w", embedErr)
		}
	}

	// Delete old chunks for this file.
	if m.vecAvailable {
		store.DeleteVecChunksByPath(m.db, entry.Path, string(source))
	}
	store.DeleteFileAndChunks(m.db, entry.Path, source, m.provider.Model(), m.ftsAvailable)

	// Insert new chunks.
	embeddingIdx := 0
	for i, chunk := range chunks {
		chunkID := uuid.New().String()

		var embeddingVec []float32
		if cached, ok := cachedEmbeddings[chunk.Hash]; ok {
			embeddingVec = meminternal.ParseEmbedding(cached)
		} else if embeddingIdx < len(newEmbeddings) {
			embeddingVec = newEmbeddings[embeddingIdx]
			embeddingIdx++

			// Cache the new embedding.
			if m.cfg.Cache.Enabled {
				embJSON, _ := json.Marshal(embeddingVec)
				store.UpsertEmbeddingCache(m.db, m.provider.ID(), m.provider.Model(), providerKey, chunk.Hash, string(embJSON), len(embeddingVec))
			}
		}

		embJSON, _ := json.Marshal(embeddingVec)
		if err := store.InsertChunk(m.db, chunkID, entry.Path, source,
			chunk.StartLine, chunk.EndLine, chunk.Hash, m.provider.Model(),
			texts[i], string(embJSON)); err != nil {
			logger.Warn("[Memory] failed to insert chunk: %v", err)
			continue
		}

		// Insert into FTS.
		if m.ftsAvailable {
			store.InsertFTSChunk(m.db, texts[i], chunkID, entry.Path, source,
				m.provider.Model(), chunk.StartLine, chunk.EndLine)
		}

		// Insert into vec0 table.
		if m.vecAvailable && len(embeddingVec) > 0 {
			store.InsertVecChunk(m.db, chunkID, embeddingVec)
		}
	}

	// Update file record.
	store.UpsertFileRecord(m.db, entry, source)

	return nil
}

// ReadFile reads a memory file and returns lines within the specified range.
// Matches OpenClaw's MemoryIndexManager.readFile().
func (m *Manager) ReadFile(path string, from, lines int) (string, error) {
	// Resolve path relative to workspace.
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(m.cfg.WorkspaceDir, path)
	}

	// Security: ensure path is under workspace.
	resolved, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	wsResolved, _ := filepath.Abs(m.cfg.WorkspaceDir)
	if len(resolved) < len(wsResolved) || resolved[:len(wsResolved)] != wsResolved {
		return "", fmt.Errorf("path %q is outside workspace", path)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	if from <= 0 && lines <= 0 {
		return string(content), nil
	}

	allLines := splitLines(string(content))
	startIdx := 0
	if from > 0 {
		startIdx = from - 1
	}
	if startIdx >= len(allLines) {
		return "", nil
	}

	endIdx := len(allLines)
	if lines > 0 {
		endIdx = startIdx + lines
		if endIdx > len(allLines) {
			endIdx = len(allLines)
		}
	}

	result := ""
	for i := startIdx; i < endIdx; i++ {
		if i > startIdx {
			result += "\n"
		}
		result += allLines[i]
	}
	return result, nil
}

// WriteMemory writes content to a memory file and triggers re-indexing.
// The path must be relative and within the memory directory (e.g., "memory/2026-02-13.md").
// If append is true, content is appended to the existing file; otherwise the file is overwritten.
func (m *Manager) WriteMemory(ctx context.Context, relPath, content string, appendMode bool) error {
	if m.closed.Load() {
		return fmt.Errorf("manager is closed")
	}

	// Security: validate path is within workspace/memory.
	absPath, err := m.resolveMemoryPath(relPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if appendMode {
		// Append with a leading newline separator.
		existing, _ := os.ReadFile(absPath)
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			content = "\n" + content
		}
		if err := os.WriteFile(absPath, append(existing, []byte(content)...), 0o644); err != nil {
			return fmt.Errorf("append file: %w", err)
		}
	} else {
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
	}

	// Mark dirty and re-sync to index the new content.
	m.dirty.Store(true)
	_ = m.Sync(ctx, SyncOpts{Reason: "memory-write"})

	return nil
}

// DeleteMemory deletes a memory file and its associated index entries.
// The path must be relative and within the memory directory.
func (m *Manager) DeleteMemory(relPath string) error {
	if m.closed.Load() {
		return fmt.Errorf("manager is closed")
	}

	absPath, err := m.resolveMemoryPath(relPath)
	if err != nil {
		return err
	}

	// Delete the file.
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}

	// Clean up index.
	store.DeleteFileAndChunks(m.db, relPath, entity.MemorySourceMemory, m.provider.Model(), m.ftsAvailable)

	return nil
}

// resolveMemoryPath validates and resolves a relative path to an absolute path
// within the workspace memory directory. Prevents directory traversal attacks.
func (m *Manager) resolveMemoryPath(relPath string) (string, error) {
	// Block obvious traversal.
	if relPath == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("path must be relative, got absolute: %q", relPath)
	}

	absPath := filepath.Join(m.cfg.WorkspaceDir, relPath)
	resolved, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	wsResolved, _ := filepath.Abs(m.cfg.WorkspaceDir)
	if !strings.HasPrefix(resolved, wsResolved+string(filepath.Separator)) && resolved != wsResolved {
		return "", fmt.Errorf("path %q escapes workspace directory", relPath)
	}

	return resolved, nil
}

// Close shuts down the manager, closes the database and file watcher.
func (m *Manager) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(m.closeCh)

	if m.watcher != nil {
		m.watcher.Close()
	}

	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// Status returns a summary of the manager's current state.
func (m *Manager) Status() ManagerStatus {
	fileCount, _ := store.CountFiles(m.db)
	chunkCount, _ := store.CountChunks(m.db)
	return ManagerStatus{
		Provider:     m.provider.ID(),
		Model:        m.provider.Model(),
		FTSAvailable: m.ftsAvailable,
		VecAvailable: m.vecAvailable,
		FileCount:    fileCount,
		ChunkCount:   chunkCount,
		Syncing:      m.syncing.Load(),
		Dirty:        m.dirty.Load(),
	}
}

// ManagerStatus holds the current state of the memory manager.
type ManagerStatus struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	FTSAvailable bool   `json:"fts_available"`
	VecAvailable bool   `json:"vec_available"`
	FileCount    int    `json:"file_count"`
	ChunkCount   int    `json:"chunk_count"`
	Syncing      bool   `json:"syncing"`
	Dirty        bool   `json:"dirty"`
}

// --- File Watcher ---

// startWatcher initializes the fsnotify file watcher for auto-sync.
// Matches OpenClaw's chokidar watcher with debounce.
func (m *Manager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	m.watcher = watcher

	// Watch memory file locations.
	memoryDir := filepath.Join(m.cfg.WorkspaceDir, "memory")
	if info, err := os.Stat(memoryDir); err == nil && info.IsDir() {
		watcher.Add(memoryDir)
	}

	memoryMD := filepath.Join(m.cfg.WorkspaceDir, "MEMORY.md")
	if _, err := os.Stat(memoryMD); err == nil {
		watcher.Add(filepath.Dir(memoryMD))
	}

	debounceMs := m.cfg.Sync.WatchDebounceMs
	if debounceMs <= 0 {
		debounceMs = 1500
	}

	go func() {
		timer := time.NewTimer(0)
		timer.Stop()
		defer timer.Stop()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
					timer.Reset(time.Duration(debounceMs) * time.Millisecond)
				}
			case <-timer.C:
				m.dirty.Store(true)
				_ = m.Sync(context.Background(), SyncOpts{Reason: "watcher"})
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			case <-m.closeCh:
				return
			}
		}
	}()

	logger.Info("[Memory] file watcher started")
	return nil
}

// embedQueryWithTimeout embeds a query with a timeout.
func (m *Manager) embedQueryWithTimeout(ctx context.Context, query string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return m.provider.EmbedQuery(ctx, query)
}

// --- Options ---

// SearchOption configures search behavior.
type SearchOption func(*entity.QueryConfig)

// WithMaxResults sets the max results for a search.
func WithMaxResults(n int) SearchOption {
	return func(cfg *entity.QueryConfig) {
		cfg.MaxResults = n
	}
}

// WithMinScore sets the minimum score threshold.
func WithMinScore(s float64) SearchOption {
	return func(cfg *entity.QueryConfig) {
		cfg.MinScore = s
	}
}

// SyncOpts holds options for a sync operation.
type SyncOpts struct {
	// Reason describes what triggered the sync.
	Reason string

	// Force forces a full reindex even if hashes match.
	Force bool
}

// --- Cache Key ---

// cacheKey generates a stable key for the manager cache.
func cacheKey(cfg *entity.MemoryConfig) string {
	return fmt.Sprintf("%s:%s:%s", cfg.WorkspaceDir, cfg.Embedding.Provider, cfg.Embedding.Model)
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	if start <= len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// ClearCache removes all cached managers. Used for testing.
func ClearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for k, m := range cache {
		m.Close()
		delete(cache, k)
	}
}

// VecAvailable returns whether the sqlite-vec extension is active.
func (m *Manager) VecAvailable() bool {
	return m.vecAvailable
}

// atomicClearIndex wipes all chunk data (chunks, FTS, vec, files) so that
// a full re-sync with the new embedding model can be performed cleanly.
// This is the "atomic rebuild" approach — clear the old index in-place,
// then the next Sync() will re-index everything with the new model.
func atomicClearIndex(db *sql.DB, ftsAvailable bool) error {
	stmts := []string{
		`DELETE FROM ` + store.TableChunks,
		`DELETE FROM ` + store.TableFiles,
		`DELETE FROM ` + store.TableEmbeddingCache,
	}
	if ftsAvailable {
		stmts = append(stmts, `DELETE FROM `+store.TableChunksFTS)
	}
	// Try to clear vec table (may not exist).
	stmts = append(stmts, `DELETE FROM `+store.TableChunksVec)

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			// Non-fatal: vec table may not exist.
			continue
		}
	}
	return nil
}

// embeddingDimensions returns the expected embedding dimension for known models.
func embeddingDimensions(model string) int {
	switch model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	case "text-embedding-004": // Gemini
		return 768
	default:
		return 1536 // safe default
	}
}
