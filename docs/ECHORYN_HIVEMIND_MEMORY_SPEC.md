# Echoryn Hivemind — 记忆系统 (Memory) 详解

> 本文档是 `ECHORYN_SPEC.md` 的子文档，深入阐述 Hivemind 中 **Memory 系统** 的完整实现逻辑。
>
> 代码位置: `internal/hivemind/service/plugin/builtin/memorycore/`

---

## 一、模块概述

Memory 系统是 Echoryn 的**内置插件** (memory-core)，为 Agent 提供长期记忆能力。它基于 SQLite 存储，实现了**混合搜索**（关键词 + 向量语义），支持 OpenAI 和 Gemini 双 Embedding Provider。

核心设计完全对齐 OpenClaw 的记忆系统，包括：
- 相同的 SQLite Schema 结构
- 相同的混合搜索算法（向量 + BM25）
- 相同的默认参数（chunk 400 token, overlap 80, minScore 0.35 等）
- 相同的文件约定（`MEMORY.md`, `memory/` 目录）

---

## 二、架构概览

```
memorycore Plugin
  │
  ├── plugin.go              # 插件入口 (Plugin + InitPlugin + LifecyclePlugin + PromptProvider)
  │   ├─ 4 个工具: memory_search / memory_read / memory_write / memory_delete
  │   ├─ 2 个 Hook: before_agent_start (记忆同步) + agent_end (记忆 flush)
  │   └─ PromptProvider: MemorySection (Priority=400)
  │
  ├── manager/manager.go     # Manager — 索引管理器核心 ★
  │   ├─ 混合搜索 (向量 + 关键词)
  │   ├─ 文件同步 + 增量索引
  │   ├─ Embedding 缓存
  │   └─ fsnotify 文件监控
  │
  ├── store/                 # SQLite 存储层
  │   ├─ schema.go           # DDL + vec0 向量索引
  │   └─ operations.go       # CRUD 操作 (参数化 SQL)
  │
  ├── embedding/             # Embedding Provider
  │   ├─ provider.go         # Provider 接口
  │   ├─ factory.go          # 工厂 (openai/gemini/auto)
  │   ├─ openai.go           # OpenAI Embeddings API
  │   └─ gemini.go           # Gemini batchEmbedContents API
  │
  ├── entity/                # 领域类型
  │   ├─ config.go           # 完整配置结构
  │   └─ types.go            # MemoryChunk/SearchResult/FileEntry
  │
  └── internal/              # 内部算法
      ├─ chunk.go            # Markdown 切片算法
      ├─ files.go            # 文件扫描 (MEMORY.md + memory/)
      ├─ hash.go             # SHA-256 哈希
      ├─ similarity.go       # 余弦相似度
      ├─ hybrid/hybrid.go    # 混合搜索合并 + FTS5 查询构建
      └─ search/search.go    # 向量搜索 + 关键词搜索
```

---

## 三、配置结构

```go
type MemoryConfig struct {
    Embedding  EmbeddingConfig   // Embedding Provider 配置
    Store      StoreConfig       // SQLite 存储配置
    Chunking   ChunkingConfig    // 文档切片配置
    Sync       SyncConfig        // 同步策略配置
    Query      QueryConfig       // 查询配置
    Cache      CacheConfig       // 缓存配置
}
```

### 3.1 默认值 (完全对齐 OpenClaw)

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| Embedding Provider | `openai` | text-embedding-3-small |
| Store Driver | `sqlite` | WAL 模式 |
| Chunk Tokens | 400 | 每块最大 token |
| Chunk Overlap | 80 | 重叠 token |
| Sync OnSessionStart | true | 会话开始时同步 |
| Sync Watch | true | fsnotify 监控 |
| Sync Debounce | 500ms | 去抖间隔 |
| Max Results | 6 | 搜索最大返回数 |
| Min Score | 0.35 | 最低相关度 |
| Vector Weight | 0.7 | 向量搜索权重 |
| Text Weight | 0.3 | 关键词搜索权重 |
| Cache MaxEntries | 10000 | Embedding 缓存上限 |

---

## 四、插件注册

### 4.1 工具 (4 个)

| 工具 | 参数 | 功能 |
|------|------|------|
| `memory_search` | query, limit?, source? | 混合搜索 (向量 + 关键词) |
| `memory_read` | path, from?, lines? | 按行范围读取记忆文件 |
| `memory_write` | path, content, append? | 写入/追加记忆文件 |
| `memory_delete` | path | 删除记忆文件 + 清理索引 |

### 4.2 Hook (2 个)

| Hook | 触发时机 | 行为 |
|------|---------|------|
| `before_agent_start` | Agent 执行前 | 同步记忆文件 + 注入记忆系统指令 |
| `agent_end` | Agent 执行后 | 提取最近对话要点，写入 `memory/YYYY-MM-DD.md` |

### 4.3 PromptProvider

`MemorySection` (Priority=400) — 注入记忆系统指令到 PromptPipeline：

```
## Memory & Personalization

You have access to a persistent memory system...
- Use `memory_search` to find relevant past information
- Use `memory_write` to save important new information
...
```

**双路径注入**: 当 PromptPipeline 激活时通过 Section 注入；否则通过 Hook 注入 legacy system message。

---

## 五、Manager — 索引管理核心

### 5.1 全局缓存

```go
var cache = map[string]*Manager{}  // 配置哈希 → Manager 单例
```

`Get(ctx, cfg)` 使用双重检查锁定确保同一配置只创建一个 Manager 实例。

### 5.2 初始化流程

```
newManager(ctx, cfg):
  ├─ 1. 创建 Embedding Provider (openai/gemini/auto)
  ├─ 2. 打开 SQLite (WAL 模式)
  ├─ 3. EnsureSchema() — 创建表和索引
  ├─ 4. 检测 provider/model 变更
  │     └─ 变更 → 原子 rebuild (清空 embedding_cache + chunks_vec)
  ├─ 5. 初始同步 Sync()
  └─ 6. 启动 fsnotify Watcher (500ms 去抖)
```

### 5.3 同步流程 (Sync)

```
Sync(ctx, opts):
  ├─ CAS 防并发 (atomic CompareAndSwap)
  ├─ listMemoryFiles() — 扫描文件
  │   ├─ MEMORY.md (根目录)
  │   ├─ memory/*.md (记忆目录)
  │   └─ extraPaths (额外路径)
  ├─ 对比 hash — 增量索引
  │   ├─ hash 相同 → 跳过
  │   └─ hash 不同 → indexFile()
  ├─ 清理过期文件记录
  └─ 修剪 embedding 缓存 (超出 maxEntries → 按 updated_at 清理)
```

### 5.4 索引单个文件 (indexFile)

```
indexFile(ctx, entry, source):
  ├─ 1. 读取文件内容
  ├─ 2. ChunkMarkdown(content, cfg) — 切片
  │     ├─ maxChars = tokens × 4
  │     ├─ 按行分割 → 累积到块
  │     ├─ 长行二次分割
  │     └─ 重叠窗口 (overlap)
  ├─ 3. Batch Embed (带缓存)
  │     ├─ 查询 embedding_cache → 命中 → 复用
  │     ├─ 未命中 → provider.EmbedBatch()
  │     └─ 结果写入缓存
  ├─ 4. 删除旧记录 (DeleteFileAndChunks)
  └─ 5. 插入新记录
      ├─ InsertChunk — 文本块
      ├─ InsertFTSChunk — FTS5 全文索引
      └─ InsertVecChunk — vec0 向量索引
```

---

## 六、搜索系统

### 6.1 混合搜索流程

```
Search(ctx, query, opts...):
  │
  ├─ 1. sync if dirty — 如有未同步的变更
  │
  ├─ 2. Embed query → 向量化
  │     └─ provider.EmbedQuery(ctx, query) → []float32
  │
  ├─ 3. 向量搜索
  │     ├─ vec0 可用 → SearchVectorVec() — sqlite-vec KNN
  │     └─ 否则 → SearchVector() — 纯 Go brute-force 余弦相似度
  │
  ├─ 4. 关键词搜索
  │     ├─ FTS5 可用 → SearchKeyword()
  │     │   ├─ BuildFTSQuery(query) — token 提取 + AND 查询
  │     │   └─ FTS5 MATCH + BM25 排序
  │     └─ 否则 → 跳过
  │
  ├─ 5. MergeResults(vector, keyword, vectorWeight, textWeight)
  │     ├─ 按 ID 合并
  │     ├─ 加权评分: score = vectorScore × vectorWeight + textScore × textWeight
  │     └─ 降序排列
  │
  ├─ 6. filter by minScore (0.35)
  └─ 7. limit by maxResults (6)
```

### 6.2 搜索算法详解

**向量搜索 (SearchVectorVec)** — sqlite-vec KNN：
```sql
SELECT rowid, distance FROM chunks_vec
WHERE embedding MATCH ? AND k = ?
ORDER BY distance
```

**向量搜索 (SearchVector)** — Brute-force 回退：
```go
for each chunk in chunks:
    score = CosineSimilarity(queryEmbedding, chunkEmbedding)
    if score > minScore → 加入结果
```

**关键词搜索 (SearchKeyword)** — FTS5：
```sql
SELECT id, path, start_line, end_line, snippet, rank
FROM chunks_fts WHERE chunks_fts MATCH ?
ORDER BY rank
```

**混合合并 (MergeResults)**:
```go
finalScore = vectorScore × 0.7 + textScore × 0.3
```

---

## 七、SQLite Schema

### 7.1 表结构 (完全对齐 OpenClaw)

```sql
-- 元数据表
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);

-- 文件记录表
CREATE TABLE IF NOT EXISTS files (
    path TEXT PRIMARY KEY,
    mtime_ms INTEGER,
    size INTEGER,
    hash TEXT,
    source TEXT DEFAULT 'memory'
);

-- 文本块表
CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    start_line INTEGER,
    end_line INTEGER,
    text TEXT NOT NULL,
    hash TEXT,
    embedding BLOB,
    source TEXT DEFAULT 'memory'
);

-- Embedding 缓存表
CREATE TABLE IF NOT EXISTS embedding_cache (
    hash TEXT PRIMARY KEY,
    embedding BLOB NOT NULL,
    updated_at INTEGER NOT NULL
);

-- FTS5 全文索引
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    id, path, start_line, end_line, snippet, source
);

-- vec0 向量索引 (如果 sqlite-vec 可用)
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_vec USING vec0(
    embedding float[N]
);
```

### 7.2 索引

```sql
CREATE INDEX idx_embedding_cache_updated_at ON embedding_cache(updated_at);
CREATE INDEX idx_chunks_path ON chunks(path);
CREATE INDEX idx_chunks_source ON chunks(source);
```

---

## 八、Embedding Provider

### 8.1 接口

```go
type Provider interface {
    ID() string                                    // "openai" | "gemini"
    Model() string                                 // 模型名
    EmbedQuery(ctx, text) ([]float32, error)       // 单文本向量化
    EmbedBatch(ctx, texts) ([][]float32, error)    // 批量向量化
}
```

### 8.2 实现

| Provider | API | 批量上限 | 默认模型 | 环境变量 |
|----------|-----|---------|---------|----------|
| **OpenAI** | POST /v1/embeddings | 2048/batch | text-embedding-3-small | `OPENAI_API_KEY` |
| **Gemini** | POST batchEmbedContents | 100/batch | text-embedding-004 | `GOOGLE_API_KEY` |

### 8.3 工厂逻辑

```
NewProvider(cfg):
  ├─ "openai" → 尝试 OpenAI → 失败 → 尝试 Gemini fallback
  ├─ "gemini" → 尝试 Gemini → 失败 → 尝试 OpenAI fallback
  └─ "auto"  → 先 OpenAI → 失败 → Gemini → 都失败 → error
```

---

## 九、Memory Flush (agent_end Hook)

会话结束时自动提取要点写入记忆：

```
agent_end Hook:
  ├─ 1. 查找最近一轮 user + assistant 消息
  ├─ 2. 格式化为 Markdown 日记条目:
  │     ## HH:MM
  │     **User**: <用户输入摘要>
  │     **Assistant**: <助手回复摘要>
  │
  ├─ 3. 写入 memory/YYYY-MM-DD.md (追加模式)
  └─ 4. 触发同步 (mark dirty → Sync)
```

---

## 十、安全机制

### 10.1 路径遍历防护

```go
func (m *Manager) resolveMemoryPath(relPath string) (string, error) {
    abs := filepath.Join(m.cfg.Store.Path, relPath)
    abs = filepath.Clean(abs)
    if !strings.HasPrefix(abs, m.cfg.Store.Path) {
        return "", fmt.Errorf("path traversal detected")
    }
    return abs, nil
}
```

`ReadFile` 和 `WriteMemory` 都使用此方法验证路径安全性。

### 10.2 SQL 注入防护

所有 SQL 查询使用参数化占位符 `?`，符合项目安全规范。

---

## 十一、与 OpenClaw 对比

### 11.1 完全对齐项

| 功能 | Echoryn | OpenClaw | 对齐状态 |
|------|---------|----------|---------|
| SQLite Schema | 5 表 (meta/files/chunks/cache/fts) | 5 表 (完全相同) | ✅ 完全一致 |
| 混合搜索 | 向量(0.7) + 关键词(0.3) | 向量(0.7) + 关键词(0.3) | ✅ 权重一致 |
| Chunk 参数 | tokens=400, overlap=80 | tokens=400, overlap=80 | ✅ 完全一致 |
| 搜索参数 | maxResults=6, minScore=0.35 | maxResults=6, minScore=0.35 | ✅ 完全一致 |
| 文件约定 | MEMORY.md + memory/ | MEMORY.md + memory/ | ✅ 完全一致 |
| 增量同步 | hash 比对 + 增量索引 | hash 比对 + 增量索引 | ✅ 逻辑一致 |
| Embedding 缓存 | embedding_cache 表 | embedding_cache 表 | ✅ 完全一致 |
| 全局实例缓存 | `map[string]*Manager` | `INDEX_CACHE` | ✅ 模式一致 |
| Markdown 切片 | ChunkMarkdown (重叠窗口) | 相同算法 | ✅ 算法一致 |

### 11.2 差异项

| 功能 | Echoryn | OpenClaw | 差异说明 |
|------|---------|----------|---------|
| **工具数量** | 4 个 (search/read/write/delete) | 2 个 (memory_search/memory_get) | **Echoryn 多出 write/delete 工具** |
| **Embedding Provider** | OpenAI + Gemini | OpenAI + Gemini + Ollama | Echoryn 缺少 Ollama embedding |
| **Memory Flush** | 简化版 (提取最近一轮写入) | LLM 驱动 (总结要点 + token 阈值) | OpenClaw 更智能 |
| **Prompt 注入** | PromptPipeline Section (P:400) | Hook 注入 system message | **Echoryn 更优雅 (插件化)** |
| **vec0 向量索引** | 支持 (sqlite-vec) | 不使用 | **Echoryn 更高效** |
| **会话记忆** | 未实现 (仅文件记忆) | sessions/ 目录 | OpenClaw 有会话级记忆 |
| **远程存储** | 不支持 | remote/batch 远程同步 | OpenClaw 支持远程 |

### 11.3 设计理念差异

| 理念 | Echoryn | OpenClaw |
|------|---------|----------|
| **记忆定位** | 插件 (可替换 slot) | 内置核心能力 |
| **注入方式** | PromptPipeline Section + Hook 双路径 | Hook 注入 |
| **搜索后端** | SQLite + vec0 (原生扩展) | SQLite + brute-force |
| **写入能力** | Agent 可主动写入记忆 | Agent 不能直接写入 (仅 flush) |
| **扩展性** | slot 互斥，可替换为其他 memory 插件 | 固定实现 |

---

## 十二、配置示例

```json
{
  "plugins": {
    "entries": {
      "memory-core": {
        "enabled": true,
        "config": {
          "embedding": {
            "provider": "openai",
            "model": "text-embedding-3-small"
          },
          "store": {
            "driver": "sqlite",
            "path": "./data/memory"
          },
          "chunking": {
            "tokens": 400,
            "overlap": 80
          },
          "query": {
            "hybrid": {
              "maxResults": 6,
              "minScore": 0.35,
              "vectorWeight": 0.7,
              "textWeight": 0.3
            }
          }
        }
      }
    }
  }
}
```

---

> 最后更新: 2026-02-13
