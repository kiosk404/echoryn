# 从 TypeScript 到 Go：AI Agent 长期记忆系统的工程实践

> 本文以 OpenClaw（TypeScript）和 Echoryn（Go）两个项目为基础，深入剖析 AI Agent 长期记忆模块的设计思路与实现细节。两者共享同一套设计哲学——**基于 SQLite 的混合搜索长期记忆**，但在工程实现上各有取舍。

---

## 一、为什么 Agent 需要长期记忆？

大语言模型（LLM）的上下文窗口是有限的。即使是 200K token 的模型，在长时间多轮对话中也会丢失早期信息。**对话压缩**（Compaction）可以延缓这个问题，但无法从根本上解决。

长期记忆系统的核心目标是：

1. **跨会话持久化** — 用户的偏好、项目上下文、历史决策不应随会话结束而消失
2. **语义检索** — 不是简单的关键词匹配，而是理解"这段记忆和当前问题有多相关"
3. **自动同步** — 记忆文件变更后自动重新索引，无需手动操作
4. **透明可控** — 记忆以 Markdown 文件形式存储，用户可以直接编辑

---

## 二、整体架构

两个项目的记忆系统都以**插件**形式存在，遵循各自的插件框架规范：

```
┌──────────────────────────────────────────────────────┐
│                    Agent Runtime                      │
│                                                      │
│  ┌─────────────┐    ┌─────────────┐                  │
│  │ Prompt       │    │ Tool        │                  │
│  │ Pipeline     │    │ Registry    │                  │
│  └──────┬──────┘    └──────┬──────┘                  │
│         │                  │                          │
│  ┌──────▼──────────────────▼──────┐                  │
│  │       memory-core Plugin       │                  │
│  │                                │                  │
│  │  ┌──────────┐  ┌───────────┐  │                  │
│  │  │  Hooks   │  │  Tools    │  │                  │
│  │  │ • before │  │ • search  │  │                  │
│  │  │ • after  │  │ • read    │  │                  │
│  │  │          │  │ • write   │  │                  │
│  │  │          │  │ • delete  │  │                  │
│  │  └────┬─────┘  └────┬─────┘  │                  │
│  │       │              │        │                  │
│  │  ┌────▼──────────────▼─────┐  │                  │
│  │  │     Index Manager       │  │                  │
│  │  │                         │  │                  │
│  │  │  ┌─────────────────┐   │  │                  │
│  │  │  │  Hybrid Search  │   │  │                  │
│  │  │  │ Vector + FTS5   │   │  │                  │
│  │  │  └────────┬────────┘   │  │                  │
│  │  │           │            │  │                  │
│  │  │  ┌────────▼────────┐   │  │                  │
│  │  │  │    SQLite DB    │   │  │                  │
│  │  │  │ WAL + FTS5 +    │   │  │                  │
│  │  │  │ vec0 (optional) │   │  │                  │
│  │  │  └────────┬────────┘   │  │                  │
│  │  │           │            │  │                  │
│  │  │  ┌────────▼────────┐   │  │                  │
│  │  │  │   Embedding     │   │  │                  │
│  │  │  │ OpenAI / Gemini │   │  │                  │
│  │  │  └────────────────┘   │  │                  │
│  │  └────────────────────────┘  │                  │
│  └───────────────────────────────┘                  │
└──────────────────────────────────────────────────────┘

文件系统:
  MEMORY.md          ← 主记忆文件
  memory/            ← 记忆目录
    2025-01-15.md
    2025-01-16.md
    project-notes.md
```

### 数据流概览

```
用户发送消息
    │
    ▼
Hook: before_agent_start
    │ → 同步记忆文件（增量索引）
    │ → 搜索相关记忆，注入 System Prompt
    ▼
Agent 执行（LLM 推理 + Tool Call 循环）
    │ → Agent 可调用 memory_search / memory_read
    │ → Agent 可调用 memory_write / memory_delete (Echoryn 独有)
    ▼
Hook: agent_end
    │ → 提取本轮对话要点
    │ → 写入 memory/YYYY-MM-DD.md
    ▼
响应返回给用户
```

---

## 三、存储层：SQLite 为什么是正确选择

两个项目不约而同地选择了 **SQLite** 作为记忆索引的存储引擎。这不是偶然——对于单用户 Agent 场景，SQLite 提供了最佳的性价比：

- **零部署成本** — 嵌入式数据库，无需额外进程
- **WAL 模式** — 读写并发，不阻塞搜索
- **FTS5 扩展** — 内置全文搜索，BM25 排序
- **vec0 扩展** — 可选的向量索引（KNN 搜索），无需 Pinecone/Qdrant 等外部服务

### 3.1 为什么需要 Markdown + 索引的双层存储？

一个自然的疑问：**既然都有向量数据库了，为什么还要用 Markdown 文件存记忆？直接写数据库不就行了？**

答案是：**Markdown 文件是"真相源"（Source of Truth），SQLite 向量索引只是"查询加速层"**。两者不是"二选一"，而是"一主一从"：

```
┌─────────────────────────────┬──────────────────────────────┐
│   Markdown 文件（主）         │   SQLite 索引（从）            │
├─────────────────────────────┼──────────────────────────────┤
│ ✅ 人类可读可编辑              │ ❌ 二进制，人类看不懂           │
│ ✅ Git 可追踪版本历史          │ ❌ 二进制 diff 无意义           │
│ ✅ 任何编辑器可打开            │ ❌ 需要专用工具                │
│ ✅ 无损保留原文               │ ❌ 只有切片+向量（有损）         │
│ ✅ Agent 直接读写             │ ✅ Agent 通过工具搜索           │
│ ❌ 搜索靠遍历（慢）           │ ✅ 向量+FTS 毫秒级搜索          │
│ ✅ 删掉索引可重建             │ ❌ 从向量无法还原原文            │
└─────────────────────────────┴──────────────────────────────┘
```

具体来说，这个设计决策基于五个理由：

**1. 索引可重建，源文件不可逆**

删掉 `.echoryn/memory/index.db`（或 OpenClaw 的 `.sqlite`），重新扫描 Markdown 即可恢复全部索引。但反过来，从向量索引**无法还原**原始 Markdown——embedding 是单向变换（高维→低维的有损压缩），切片后上下文关系也丢失了。

```
MEMORY.md  →  切片  →  embedding  →  SQLite
   ✅           ✅         ✅           ✅     正向：可以
   ❌           ❌         ❌           ✅     反向：不可能
```

**2. 用户需要直接编辑记忆**

设计目标之一是"透明可控"。用户用任何文本编辑器打开 `MEMORY.md`，就能修改错误记忆、删除敏感信息、重新组织内容。如果记忆只存在向量库里，用户面对的是一堆浮点数——完全无法操作。

```bash
# 用户发现 Agent 记错了一个偏好，直接改文件
vim MEMORY.md   # 删除或修正那一行
# → fsnotify 检测到变更 → 增量重新索引 → 完成
```

**3. 多工具协作的不同访问模式**

记忆文件同时服务于三种工具，各自以不同方式访问：

| 工具 | 访问方式 | 为什么需要文件 |
|------|----------|-------------|
| `memory_search` | 走 SQLite 向量+FTS 索引 | 语义搜索，不需要文件 |
| `memory_read` | **直接读文件内容** | 需要原始文件才能返回完整上下文 |
| `memory_write` | **直接追加/写入文件** | 写文件是原子操作，比写数据库简单 |

如果只有数据库，`memory_read` 返回的是切片碎片而非完整文档，`memory_write` 需要同时维护文件切片、embedding、FTS 索引的一致性——复杂度暴增。

**4. 版本控制与审计**

Markdown 文件天然适合 Git：

```bash
git log --oneline memory/
# a1b2c3d 2026-02-27 记录了用户偏好 Go > Rust
# d4e5f6g 2026-02-26 修正了项目架构描述
# ...
```

用户的记忆有完整的变更历史、可以回滚、可以在多设备间同步。SQLite 的 WAL 文件做不到这些。

**5. 写入路径的简单性**

Agent 写记忆时，本质上就是往文件追加一段 Markdown 文本——这是最简单、最不容易出错的 I/O 操作。文件系统负责持久化，fsnotify 负责触发异步索引。如果直接写数据库，需要在写入时同时完成切片、embedding、插入 chunks 表、更新 FTS 索引——任何一步失败都需要事务回滚。

```
写文件路径:  Agent → appendFile("memory/2026-02-27.md", text)  → 完成
                                                        ↓ (异步)
                                                    fsnotify → 增量索引

写数据库路径: Agent → chunk(text) → embed(chunks) → BEGIN TX
                  → INSERT chunks → INSERT FTS → INSERT vec → COMMIT
                  → 任何一步失败都要 ROLLBACK
```

**总结**：Markdown 文件是"持久、可读、可编辑的记忆本体"，SQLite 索引是"高速、可丢弃、可重建的搜索缓存"。这种分离让系统既有高效的语义搜索能力，又保持了对用户完全透明可控。

### Schema 设计

两个项目使用**完全一致**的 5 表结构：

```sql
-- 元信息（Provider/Model 变更检测）
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);

-- 文件级索引（增量同步的核心）
CREATE TABLE IF NOT EXISTS files (
    path      TEXT NOT NULL,
    source    TEXT NOT NULL DEFAULT 'memory',
    hash      TEXT NOT NULL,
    indexed_at INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (path, source)
);

-- 文本块（切片后的实际内容 + 向量嵌入）
CREATE TABLE IF NOT EXISTS chunks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path  TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT 'memory',
    start_line INTEGER NOT NULL,
    end_line   INTEGER NOT NULL,
    text       TEXT NOT NULL,
    hash       TEXT NOT NULL,
    embedding  TEXT,        -- JSON 编码的 float32 数组
    model      TEXT,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Embedding 缓存（避免重复调用 API）
CREATE TABLE IF NOT EXISTS embedding_cache (
    hash         TEXT NOT NULL,
    provider_key TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    embedding    TEXT NOT NULL,
    updated_at   INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (hash, provider_key, model)
);

-- FTS5 全文搜索虚拟表
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    text,
    chunk_id UNINDEXED,
    file_path UNINDEXED,
    source UNINDEXED
);

-- 可选：vec0 向量索引
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_vec USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding float[1536]
);
```

### Provider/Model 变更检测

一个关键的工程细节：当用户切换 Embedding Provider 或 Model 时，所有已有的向量嵌入都会**失效**（不同模型的向量空间不同，混合使用会导致搜索结果完全错乱）。

两个项目的处理方式一致：

```go
// Echoryn (Go) — manager.go
oldProvider, _ := store.GetMeta(m.db, "provider")
oldModel, _ := store.GetMeta(m.db, "model")
newKey := embedding.ProviderKey(m.provider)

if oldProvider != "" && (oldProvider != m.provider.ID() || oldModel != m.provider.Model()) {
    log.Warn("Embedding provider/model changed, clearing index",
        "old", oldProvider+":"+oldModel,
        "new", newKey)
    m.atomicClearIndex()  // 清空 chunks + FTS + vec，保留 files 表
}
```

```typescript
// OpenClaw (TS) — manager.ts
const oldProviderKey = this.db.prepare('SELECT value FROM meta WHERE key = ?').get('provider_key');
if (oldProviderKey && oldProviderKey !== this.providerKey) {
    this.clearAllEmbeddings();  // 同样的逻辑
}
```

---

## 四、切片算法：从 Markdown 到可搜索的块

记忆以 Markdown 文件形式存储，但搜索需要更细粒度的单元。切片算法负责将长文档切分为**重叠的文本块**：

```
┌─────────────────────────────┐
│       MEMORY.md (500 行)     │
└──────────────┬──────────────┘
               │ ChunkMarkdown(content, cfg)
               ▼
┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐
│Chunk1│ │Chunk2│ │Chunk3│ │Chunk4│
│L1-L30│ │L25-55│ │L50-80│ │L75-99│
│      │ │      │ │      │ │      │
│ hash │ │ hash │ │ hash │ │ hash │
└──────┘ └──────┘ └──────┘ └──────┘
    ↕         ↕         ↕
  overlap   overlap   overlap
```

### 默认参数（对齐 OpenClaw）

| 参数 | 值 | 含义 |
|------|-----|------|
| `tokens` | 400 | 每块最大 token 数 |
| `overlap` | 80 | 块间重叠 token 数 |
| `maxChars` | `tokens × 4` = 1600 | 近似字符上限 |
| `overlapChars` | `overlap × 4` = 320 | 近似重叠字符量 |

### Go 实现

```go
// internal/memorycore/internal/chunk.go
func ChunkMarkdown(content string, cfg entity.ChunkingConfig) []entity.MemoryChunk {
    maxChars := max(32, cfg.Tokens*4)
    overlapChars := max(0, cfg.Overlap*4)

    lines := strings.Split(content, "\n")
    var buf []string
    var bufLen, startLine int

    flush := func() {
        if bufLen == 0 { return }
        text := strings.Join(buf, "\n")
        chunks = append(chunks, entity.MemoryChunk{
            StartLine: startLine,
            EndLine:   startLine + len(buf) - 1,
            Text:      text,
            Hash:      HashString(text),
        })
    }

    carryOverlap := func() {
        // 从末尾保留 overlapChars 字符量的行到下一个块
        // 滑动窗口保证块间信息连续性
    }

    for i, line := range lines {
        // 超长行按 maxChars 切分
        // 逐行累积，超出 maxChars 则 flush + carryOverlap
    }
    flush() // 处理残余
    return chunks
}
```

### 为什么需要重叠？

如果一段关键信息恰好跨越两个块的边界，没有重叠就会导致两个块都只包含部分信息，降低搜索精度。80 token 的重叠是在"信息完整性"和"存储/索引开销"之间的平衡点。

---

## 五、Embedding：向量化记忆

将文本块转换为高维向量是语义搜索的基础。两个项目都采用 **Provider 抽象 + 工厂模式 + 自动回退**的设计。

### 接口定义

```go
// Echoryn (Go) — embedding/provider.go
type Provider interface {
    ID() string                                    // "openai" | "gemini"
    Model() string                                 // "text-embedding-3-small"
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
```

```typescript
// OpenClaw (TS) — embeddings.ts
export interface EmbeddingProvider {
    id: string;
    model: string;
    generateEmbedding(text: string): Promise<number[]>;
    generateEmbeddings(texts: string[]): Promise<number[][]>;
}
```

### 工厂模式 + Fallback 链

```go
// Echoryn (Go) — embedding/factory.go
func NewProvider(cfg entity.EmbeddingConfig) (*ProviderResult, error) {
    switch cfg.Provider {
    case "openai":
        p, err := newOpenAI(cfg)
        if err != nil && cfg.Fallback != "none" {
            return tryFallback("gemini", cfg)
        }
        return p, err
    case "gemini":
        p, err := newGemini(cfg)
        if err != nil && cfg.Fallback != "none" {
            return tryFallback("openai", cfg)
        }
        return p, err
    case "auto":
        // 先试 OpenAI，失败再试 Gemini
    }
}
```

### 批量处理与分批

OpenAI 和 Gemini 的批量嵌入 API 都有上限。两个项目的处理方式：

| Provider | OpenClaw (TS) | Echoryn (Go) |
|----------|--------------|--------------|
| OpenAI | 2048 条/批 | 2048 条/批 |
| Gemini | 100 条/批 | 100 条/批 |
| 本地 GGUF | 支持 (node-llama-cpp) | 不支持 |

### Embedding 缓存

为避免重复调用昂贵的 API，两个项目都实现了**四元组缓存**：

```
缓存 Key = (content_hash, provider_id, model, provider_key)
```

当同一段文本在文件修改后内容不变（hash 相同），直接从缓存读取向量，省去一次 API 调用。缓存上限默认 10000 条，超出时按 `updated_at` 淘汰最旧的。

---

## 六、混合搜索：向量 + 关键词的最佳组合

纯向量搜索擅长语义理解但对精确关键词不敏感；纯关键词搜索擅长精确匹配但无法理解同义词。**混合搜索**结合两者的优势。

### 搜索流程

```
查询: "如何配置 CORS 跨域?"
    │
    ├──→ 向量搜索 (权重 0.7)
    │     │ embed("如何配置 CORS 跨域?") → [0.12, -0.03, ...]
    │     │ 
    │     ├─ 优先: vec0 KNN (sqlite-vec 扩展)
    │     │   SELECT chunk_id, distance FROM chunks_vec
    │     │   WHERE embedding MATCH ? ORDER BY distance LIMIT 18
    │     │
    │     └─ 回退: brute-force 余弦相似度
    │         for chunk in all_chunks:
    │           score = cosine(query_vec, chunk.embedding)
    │
    ├──→ 关键词搜索 (权重 0.3)
    │     │ BuildFTSQuery("如何配置 CORS 跨域?")
    │     │   → '"CORS" AND "跨域"'
    │     │
    │     SELECT *, bm25(chunks_fts) as rank
    │     FROM chunks_fts
    │     WHERE chunks_fts MATCH '"CORS" AND "跨域"'
    │     ORDER BY rank LIMIT 18
    │
    └──→ MergeResults()
          │ 按 chunk_id 合并
          │ finalScore = 0.7 × vectorScore + 0.3 × textScore
          │ 过滤: score >= 0.35 (minScore)
          │ 截断: top 6 (maxResults)
          ▼
        [结果1, 结果2, ..., 结果6]
```

### 分数归一化

两种搜索返回的分数量纲不同，需要归一化到 [0, 1] 区间：

```go
// 向量搜索: 余弦相似度天然在 [-1, 1]，通常 > 0

// FTS5 BM25 rank → [0, 1]
func BM25RankToScore(rank float64) float64 {
    return 1.0 / (1.0 + math.Max(0, rank))
}

// vec0 distance (L2) → [0, 1]
score = 1.0 / (1.0 + distance)
```

### 优雅降级策略

这是两个项目共同的工程智慧——**不因为可选组件不可用而阻塞核心功能**：

| 组件 | 可用时 | 不可用时 |
|------|--------|---------|
| FTS5 | 关键词搜索正常工作 | 跳过关键词搜索，仅用向量 |
| vec0 | KNN 加速向量搜索 | 回退到 brute-force 余弦相似度 |
| Embedding Provider | 向量搜索正常 | 记忆系统完全降级（仅文件存储） |

---

## 七、搜索增强三件套：从"能搜到"到"搜得准"

基础的混合搜索已经比纯向量或纯关键词好很多了，但在真实使用中还有三个明显的问题：

1. **结果同质化** — 如果记忆中有多段描述同一个 API 的内容，搜索结果前 6 条可能全是"CORS 配置"的不同切片，用户想看到的其他相关信息被挤出去了
2. **时间无关性** — 一条一年前写的笔记和今天写的笔记，如果语义相似度相同，搜索排序完全一样。但用户通常更想看到"最近讨论过的"
3. **CJK 搜索差** — 中文"之前讨论的那个方案"经过 BM25 匹配时，大部分 token 都是停用词，搜索命中率极低

这三个问题分别由 **MMR 重排序**、**时间衰减**、**查询扩展** 解决。OpenClaw 首先实现了这三个特性，Echoryn 随后用 Go 完整移植。

### 7.1 MMR 重排序：为什么搜索结果需要"去重"？

**问题场景**：

```
用户: "CORS 跨域怎么配置?"

搜索结果（无 MMR）:
  1. [0.92] memory/2026-01-15.md: "CORS 配置: Access-Control-Allow-Origin..."
  2. [0.90] memory/2026-01-20.md: "CORS 跨域问题排查: 修改 Access-Control-..."
  3. [0.88] memory/2026-02-01.md: "跨域配置总结: CORS headers 包括..."
  4. [0.85] MEMORY.md: "项目使用 CORS 中间件，配置..."
  5. [0.83] memory/2026-01-15.md: "CORS 预检请求 OPTIONS 处理..."
  6. [0.80] memory/2026-02-10.md: "CORS 与 Cookie 的交互..."

→ 6 条结果几乎都是同一个主题的不同片段。
  如果用户同时想了解 "API 认证" 或 "路由配置"，这些完全被淹没了。
```

**MMR（Maximal Marginal Relevance）** 通过在选择每条结果时引入**多样性惩罚**来解决这个问题。核心公式来自 Carbonell & Goldstein 1998 年的论文：

```
MMR(d) = λ × Relevance(d) - (1 - λ) × max[Similarity(d, selected)]
```

其中：
- `λ` 控制 **相关性 vs 多样性** 的平衡：λ=1 退化为纯相关性排序，λ=0 完全追求多样性
- `Relevance(d)` 是混合搜索给出的归一化分数
- `Similarity(d, selected)` 是候选文档与**已选中文档**的最大相似度

**默认 λ=0.7**，意味着 70% 权重给相关性，30% 权重给多样性。

#### 算法流程

```
输入: N 个搜索结果（已排序）

Step 1: 分数归一化到 [0, 1]
  normalizedScore = (score - minScore) / scoreRange
  如果所有分数相等 → 统一为 1

Step 2: 预分词
  对每个结果的文本提取 token: /[a-z0-9_]+/g → Set<string>
  缓存到 tokenCache 避免重复计算

Step 3: 贪心迭代选择
  selected = []
  remaining = {0, 1, ..., N-1}

  REPEAT N 次:
    对每个 remaining 中的候选 d:
      relevance = normalizedScore[d]
      maxSim = max{ Jaccard(tokens[d], tokens[s]) | s ∈ selected }
      mmr = λ × relevance - (1-λ) × maxSim

    选择 mmr 最高的候选（平分时取原始 score 更高的作为 tiebreaker）
    从 remaining 移到 selected
```

#### 为什么用 Jaccard 相似度而不是余弦相似度？

这是一个务实的设计选择。MMR 在**搜索结果之间**计算相似度，而不是在搜索结果与查询之间。文本级的 Jaccard 相似度有三个优势：

1. **无需额外 embedding** — 不需要再调一次 API
2. **计算快** — 集合交集/并集操作，O(min(|A|, |B|))
3. **对"同源内容"敏感** — 来自同一文件不同切片的内容，词汇重叠度高，Jaccard 能有效捕捉

```
Jaccard(A, B) = |A ∩ B| / |A ∪ B|

A = {cors, config, access, control, allow, origin}
B = {cors, 跨域, 配置, access, control, headers}

|A ∩ B| = 3 (cors, access, control)
|A ∪ B| = 9

Jaccard = 3/9 = 0.33 → 有一定相似，但不太高，MMR 惩罚适中
```

#### Go 实现要点

```go
// Echoryn (Go) — internal/hybrid/mmr.go

// 遍历较小集合优化
func jaccardSimilarity(setA, setB map[string]struct{}) float64 {
    smaller, larger := setA, setB
    if len(setA) > len(setB) {
        smaller, larger = setB, setA
    }
    intersectionSize := 0
    for token := range smaller {
        if _, ok := larger[token]; ok {
            intersectionSize++
        }
    }
    unionSize := len(setA) + len(setB) - intersectionSize
    return float64(intersectionSize) / float64(unionSize)
}

// MMR 核心循环
func ApplyMMR(results []HybridResult, cfg MMRConfig) []HybridResult {
    lambda := math.Max(0, math.Min(1, cfg.Lambda))
    if lambda == 1 { return results }  // 无多样性惩罚，直接返回

    // 预分词 + 归一化 + 贪心选择
    // ...每轮选 MMR score 最高的，平分时用原始 score 做 tiebreaker
}
```

#### 效果对比

```
搜索结果（有 MMR, λ=0.7）:
  1. [0.92] memory/2026-01-15.md: "CORS 配置: Access-Control-Allow-Origin..."
  2. [0.85] MEMORY.md: "项目使用 CORS 中间件，配置..."
  3. [0.78] memory/2026-02-05.md: "API 认证: JWT token 验证流程..."
  4. [0.75] memory/2026-01-20.md: "CORS 跨域问题排查..."
  5. [0.72] memory/2026-02-10.md: "路由配置: Gin middleware chain..."
  6. [0.70] memory/2026-02-10.md: "CORS 与 Cookie 的交互..."

→ 第 3、5 条是"非 CORS"的相关信息，被 MMR 从低位提拔上来
```

### 7.2 时间衰减：记忆应该"老化"

**问题场景**：2025 年 1 月写的一条记忆"项目使用 Express 框架"和 2026 年 2 月写的"项目已迁移到 Gin 框架"，如果语义相似度都是 0.85，搜索结果中两条并列。但旧信息已经**过时**了。

时间衰减通过**指数衰减函数**让旧文档的分数随时间下降：

```
decayedScore = score × e^(-λ × ageInDays)

其中 λ = ln(2) / halfLifeDays
```

当 `halfLifeDays = 30` 时：
- 1 天前的文档：score × 0.977 ≈ 几乎不变
- 7 天前：score × 0.852
- 30 天前：score × 0.500（半衰期，分数减半）
- 90 天前：score × 0.125（八分之一）
- 180 天前：score × 0.016（几乎清零）

#### 时间戳来源的优先级

文档的"年龄"从哪里获取？两个项目使用相同的三级策略：

| 优先级 | 来源 | 示例 |
|-------|------|------|
| 1 | 路径日期 | `memory/2026-01-15.md` → 2026-01-15 |
| 2 | 常青检测 | `MEMORY.md`、`memory/api-design.md` → **不衰减** |
| 3 | 文件 mtime | `os.Stat(path).ModTime()` |

**常青文件（Evergreen）** 的概念很关键——有些记忆文件是**永久有效**的：

```
MEMORY.md                → 常青（根级记忆文件，核心偏好）
memory/api-design.md     → 常青（无日期的主题文件）
memory/2026-01-15.md     → 有日期，会衰减
memory/2026-02-27.md     → 有日期，最近的，衰减很小
```

判断规则：source 为 `"memory"` 的文件中，`MEMORY.md`/`memory.md` 或 `memory/` 目录下不含 `YYYY-MM-DD` 的文件，视为常青。

```go
// Echoryn (Go) — internal/hybrid/temporal_decay.go

func isEvergreenMemoryPath(filePath string) bool {
    if normalized == "MEMORY.md" || normalized == "memory.md" {
        return true
    }
    if !strings.HasPrefix(normalized, "memory/") {
        return false
    }
    // memory/ 下但不是 YYYY-MM-DD.md → 常青
    return !datedMemoryPathRE.MatchString(normalized)
}
```

#### 时间戳缓存

同一文件的多个切片（chunk）共享时间戳。为避免对同一文件重复 `os.Stat()`，使用 `source:path` 为 key 的缓存：

```go
timestampCache := make(map[string]*time.Time)
cacheKey := string(r.Source) + ":" + r.Path
```

OpenClaw 的 TypeScript 版本使用 `Promise<Date | null>` 缓存，因为 `fs.stat()` 是异步的。Go 版本直接缓存 `*time.Time`。

### 7.3 查询扩展：让中文搜索不再是噩梦

**问题场景**：

```
用户查询: "之前讨论的那个方案"

FTS5 搜索: MATCH '"之前" AND "讨论" AND "的" AND "那个" AND "方案"'
→ 大部分 token 是停用词（之前、的、那个），FTS5 几乎匹配不到有意义的结果
```

查询扩展做两件事：
1. **去除停用词** — 从 7 种语言的停用词表中过滤无意义 token
2. **CJK 分词** — 将中文/日文汉字拆分为 unigram + bigram，提高匹配率

#### 7 语言停用词表

| 语言 | 词数 | 示例 |
|------|------|------|
| 英语 | ~70 | a, the, is, was, this, that, help, find, show |
| 中文 | ~76 | 我、的、了、是、在、有、这个、那个、之前、什么 |
| 日语 | ~32 | これ、する、です、の、こと、なぜ、今日 |
| 韩语 | ~87 | 은, 는, 이, 가, 있다, 없다, 하다, 왜, 어떻게 |
| 西班牙语 | ~50 | el, la, un, de, en, es, que, como |
| 葡萄牙语 | ~45 | o, a, um, de, em, é, que, como |
| 阿拉伯语 | ~45 | ال, و, من, في, على, هذا, كان, لماذا |

#### 中文分词策略

中文没有空格分词，FTS5 的默认 tokenizer 对中文几乎无用。两个项目采用**字符级 unigram + bigram** 策略：

```
输入: "讨论方案"

Unigrams: ["讨", "论", "方", "案"]
Bigrams:  ["讨论", "论方", "方案"]

合并后: ["讨", "论", "方", "案", "讨论", "论方", "方案"]
→ FTS 查询: "讨论方案 OR 讨 OR 论 OR 方 OR 案 OR 讨论 OR 方案"
```

Bigram 的价值在于：如果记忆中包含"讨论了技术方案"，unigram "方" 会匹配到很多无关内容（"方法"、"地方"），但 bigram "方案" 精确匹配了真正相关的文本。

#### 日文分词：混合脚本分离

日文文本经常混合 ASCII、片假名、汉字、平假名四种脚本，需要按脚本类型分割后分别处理：

```
输入: "APIのデザインパターン設計"

分段:
  ASCII:    "api"
  平假名:   "の" → 太短(1字)，跳过
  片假名:   "デザインパターン" → 保留整段
  汉字:     "設計" → unigram ["設", "計"] + bigram ["設計"]
```

#### 韩文助词剥离

韩文的黏着语特性导致同一个词根会附着不同的助词：

```
"API를" (API+宾格助词) → 剥离 "를" → 词根 "API"
"프로젝트에서" (项目+位格助词) → 剥离 "에서" → 词根 "프로젝트"
```

两个项目都实现了 **26 个韩文尾随助词**的最长匹配剥离，并要求剥离后的韩文词根至少 2 个音节（防止过度剥离）：

```go
// Go 实现
var koTrailingParticles = []string{
    "에서", "으로", "에게", "한테", "처럼", "같이", "보다", "까지", "부터", "마다", "밖에", "대로",
    "은", "는", "이", "가", "을", "를", "의", "에", "로", "와", "과", "도", "만",
}
```

#### 扩展后的 FTS 查询

```
原始: "之前讨论的那个方案"

过滤停用词: "之前"✗ "讨论"→拆分 "的"✗ "那个"✗ "方案"→拆分

Keywords: ["讨", "论", "讨论", "方", "案", "方案"]

扩展查询: "之前讨论的那个方案 OR 讨 OR 论 OR 讨论 OR 方 OR 案 OR 方案"
```

这样即使原始查询匹配不上，"讨论" 和 "方案" 这两个 bigram 有很大概率命中相关记忆。

### 7.4 三件套协作：完整的搜索管线

这三个增强功能的执行顺序很重要：

```
用户查询
    │
    ▼
[1] 查询扩展（仅 FTS 路径）
    │  "之前讨论的方案" → "之前讨论的方案 OR 讨论 OR 方案"
    │
    ├──→ 向量搜索 (权重 0.7)
    │     embed(原始查询) → KNN / brute-force
    │
    ├──→ 关键词搜索 (权重 0.3)
    │     使用扩展后的 FTS 查询
    │
    └──→ MergeResults
          │
          ├── [2] 加权融合: 0.7 × vectorScore + 0.3 × textScore
          │
          ├── [3] 时间衰减: score × e^(-λ × ageDays)
          │        → 旧文档分数降低，但常青文件不受影响
          │
          ├── [4] 按分数排序
          │
          └── [5] MMR 重排序: λ × relevance - (1-λ) × diversity_penalty
                   → 去除过于相似的结果，提升多样性
          │
          ▼
        过滤 minScore + 截断 maxResults
          │
          ▼
        最终结果
```

注意执行顺序：**时间衰减在排序之前**（改变分数），**MMR 在排序之后**（改变顺序）。这是因为时间衰减影响的是绝对分数，而 MMR 是在已排序的候选集上做多样性选择。

### 7.5 配置一览

所有增强功能默认**关闭**（opt-in），通过配置开启：

```go
// Echoryn (Go) — entity/types.go
type QueryConfig struct {
    MaxResults    int              `json:"max_results"`     // 默认 6
    MinScore      float64          `json:"min_score"`       // 默认 0.35
    Hybrid        HybridConfig     `json:"hybrid"`          // 向量 0.7 / 文本 0.3
    MMR           MMRConfig        `json:"mmr"`             // 默认关闭, λ=0.7
    TemporalDecay TemporalDecayConfig `json:"temporal_decay"` // 默认关闭, 半衰期 30 天
}
```

---

## 八、记忆工具：Agent 如何操作记忆

记忆系统的工具是 Agent 操作记忆的唯一入口。两个项目在工具设计上有显著差异。

### 8.1 OpenClaw 的工具体系

OpenClaw 的 memory-core 插件暴露 **2 个工具**：

| 工具 | 参数 | 用途 |
|------|------|------|
| `memory_search` | `query`（必填）, `maxResults`, `minScore` | 语义搜索 MEMORY.md + memory/*.md |
| `memory_get` | `path`（必填）, `from`（行号）, `lines`（行数） | 读取文件指定片段 |

**没有 write 和 delete** — OpenClaw 认为记忆写入应该通过 Hook 机制自动完成（见后文 Memory Flush 和 Session Memory Hook），而非让 Agent 直接写文件。

`memory_search` 的返回值包含丰富的元数据：
```typescript
{
  results: SearchResult[],
  provider: "openai",           // 当前 embedding provider
  fallback: false,              // 是否在用 fallback provider
  searchMode: "hybrid",         // hybrid | fts-only
  citations: "on" | "off"       // 是否显示来源引用
}
```

`memory_get` 有严格的**安全检查**：
- 路径必须在 workspace 范围内
- 必须是 `memory/` 下的路径或已配置的 `extraPaths`
- 只允许 `.md` 文件
- 支持行级切片（`from` + `lines`），避免一次性加载大文件

### 8.2 OpenClaw LanceDB 扩展的工具体系

LanceDB 扩展是**独立于 memory-core 的另一套记忆系统**，使用 LanceDB 向量数据库替代 SQLite，暴露 **3 个工具**：

| 工具 | 参数 | 用途 |
|------|------|------|
| `memory_recall` | `query` | 向量搜索 LanceDB 中的记忆 |
| `memory_store` | `text`, `importance`, `category` | 存储新记忆（去重：相似度 > 0.95 跳过） |
| `memory_forget` | `id` 或 `query` | 删除记忆（GDPR 合规） |

LanceDB 扩展的独特设计：
- **Auto-Recall**（before_agent_start Hook）：每轮对话开始前，自动将用户消息嵌入向量搜索 LanceDB，将前 3 条结果注入 `<relevant-memories>` XML 块
- **Auto-Capture**（agent_end Hook）：对话结束后，分析**仅用户消息**（避免模型输出自中毒），使用规则引擎判断是否值得保存
- **提示注入防护**：`looksLikePromptInjection()` 拒绝包含 "ignore previous instructions"、"system prompt" 等模式的内容
- **UUID 验证**：删除操作使用正则验证 ID 格式，防止 SQL 注入

### 8.3 Echoryn 的工具体系

Echoryn 暴露 **4 个工具**：

| 工具 | 参数 | 用途 |
|------|------|------|
| `memory_search` | `query`, `maxResults`, `minScore` | 混合搜索（向量 + FTS5） |
| `memory_read` | `path`, `startLine`, `endLine` | 读取文件指定行范围 |
| `memory_write` | `path`, `content`, `mode`(append/overwrite) | 写入记忆文件 |
| `memory_delete` | `path` | 删除记忆文件 |

与 OpenClaw 不同，Echoryn 选择让 Agent **直接操作文件**。这背后的设计哲学是"教 Agent 钓鱼"——Agent 自己判断何时写、写什么、写到哪里。

### 8.4 工具设计哲学对比

| 维度 | OpenClaw memory-core | OpenClaw LanceDB | Echoryn |
|------|---------------------|-------------------|---------|
| 搜索 | ✅ memory_search | ✅ memory_recall | ✅ memory_search |
| 读取 | ✅ memory_get | ❌（内置 auto-recall） | ✅ memory_read |
| 写入 | ❌（Hook 自动） | ✅ memory_store | ✅ memory_write |
| 删除 | ❌ | ✅ memory_forget | ✅ memory_delete |
| 自动召回 | ❌ | ✅ Auto-Recall Hook | ❌ |
| 自动捕获 | ❌ | ✅ Auto-Capture Hook | ❌ |

---

## 九、增量同步：只索引变更的内容

全量重建索引对于大量记忆文件来说太慢。两个项目都实现了基于 **SHA-256 哈希**的增量同步：

```
同步触发 (before_agent_start / fsnotify / 手动)
    │
    ▼
ListMemoryFiles(workspaceDir)
    │ → 扫描 MEMORY.md, memory/, extraPaths
    │ → 跳过符号链接、非常规文件
    │ → 按 realpath 去重
    ▼
对每个文件:
    │ 计算 SHA-256 hash
    │ 对比 files 表中的旧 hash
    │
    ├─ hash 相同 → 跳过（标记为 seen）
    │
    └─ hash 不同 / 新文件 → indexFile()
         │ 读取文件内容
         │ ChunkMarkdown() 切片
         │ 查 embedding_cache（命中则复用）
         │ EmbedBatch() 批量嵌入未缓存的块
         │ 删除旧的 chunks / FTS / vec 记录
         │ 插入新记录
         ▼
清理: 删除不再存在的文件记录
修剪: embedding_cache 超过 maxEntries 时淘汰旧条目
```

### 并发控制

Go 版本使用 `atomic.Bool` 的 CAS（Compare-And-Swap）操作防止同步并发：

```go
func (m *Manager) Sync(ctx context.Context, opts SyncOpts) error {
    if !m.syncing.CompareAndSwap(false, true) {
        return nil  // 已有同步在进行，直接返回
    }
    defer m.syncing.Store(false)
    // ... 执行同步
}
```

### fsnotify 文件监控 + 去抖

```go
func (m *Manager) startWatcher() {
    timer := time.NewTimer(math.MaxInt64)  // 初始不触发
    
    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if isMemoryPath(event.Name) {
                    m.dirty.Store(true)
                    timer.Reset(debounceMs)  // 去抖：重置定时器
                }
            case <-timer.C:
                m.Sync(ctx, SyncOpts{Reason: "fsnotify"})
            }
        }
    }()
}
```

去抖的意义：用户可能连续编辑多个文件，我们不需要每次保存都触发索引，而是等待一个安静期（默认 1500ms）后一次性同步。

---

## 十、记忆 Hook 系统：自动化记忆生命周期

记忆系统不仅需要"搜索"和"写入"工具，还需要在**关键时刻自动触发**记忆操作。这就是 Hook 系统的作用——在会话生命周期的特定节点，自动执行记忆相关的逻辑。

### 10.1 OpenClaw 的 Session Memory Hook

OpenClaw 实现了一个精巧的 Session Memory Hook——当用户执行 `/new`（新建会话）或 `/reset`（重置会话）时，自动将当前会话的关键内容保存为长期记忆文件。

```typescript
// OpenClaw — session-memory/handler.ts
// 触发事件：command:new, command:reset

async function handleSessionMemory(event: HookEvent): Promise<void> {
    // 1. 读取当前 session 的 JSONL 文件
    const sessionPath = getSessionPath(event.sessionId);
    const messages = parseSessionMessages(sessionPath);
    
    // 2. 提取最近 N 条消息（默认 15 条）
    const recentMessages = messages
        .filter(m => m.role === 'user' || m.role === 'assistant')
        .slice(-MAX_MESSAGES);  // MAX_MESSAGES = 15
    
    // 3. 使用 LLM 生成描述性 slug
    const slug = await generateSlug(recentMessages);
    // 例如: "refactoring-auth-middleware" / "debugging-memory-leak"
    
    // 4. 格式化为 Markdown，写入记忆文件
    const filename = `${dateStr}-${slug}.md`;  // 2026-02-28-refactoring-auth-middleware.md
    const content = formatAsMarkdown(recentMessages);
    await writeFile(`memory/${filename}`, content);
}
```

**核心设计决策：**

| 决策 | 选择 | 原因 |
|------|------|------|
| 触发时机 | `/new` 和 `/reset` | 会话切换是天然的"记忆沉淀"节点 |
| 消息数量 | 最近 15 条 | 平衡完整性和成本（太多 → LLM 处理慢，太少 → 丢失上下文） |
| 文件命名 | `YYYY-MM-DD-slug.md` | 日期 + 语义 slug，兼顾时间排序和可读性 |
| Slug 生成 | LLM 驱动 | 比简单截取标题更准确，能概括整个会话主题 |
| 消息过滤 | 仅 user + assistant | 跳过 tool 消息（太冗长，且搜索时用处不大） |

### 10.2 OpenClaw 的 Session Transcript 事件

除了 Session Memory Hook，OpenClaw 还有一套更底层的 **Session Transcript** 机制——实时监听会话消息变化，用于 Session 级别的持久化：

```typescript
// OpenClaw — session transcript delta detection
// 监听消息写入事件，5s 去抖

class SessionTranscriptListener {
    private debounceTimer: NodeJS.Timeout | null = null;
    private lastProcessedIndex = 0;
    
    onMessageAppended(sessionId: string) {
        // 去抖：5 秒内多次写入只处理一次
        if (this.debounceTimer) clearTimeout(this.debounceTimer);
        this.debounceTimer = setTimeout(() => {
            this.processDeltas(sessionId);
        }, 5000);
    }
    
    private async processDeltas(sessionId: string) {
        const messages = readSessionMessages(sessionId);
        const newMessages = messages.slice(this.lastProcessedIndex);
        
        if (newMessages.length > 0) {
            await this.exportToMarkdown(sessionId, newMessages);
            this.lastProcessedIndex = messages.length;
        }
    }
}
```

Transcript 和 Session Memory Hook 的区别：

| 维度 | Session Memory Hook | Session Transcript |
|------|-------------------|-------------------|
| 触发时机 | 用户执行 `/new` / `/reset` | 每次消息写入（5s 去抖） |
| 输出 | `memory/YYYY-MM-DD-slug.md` | Session 导出文件 |
| 用途 | 长期记忆沉淀 | 会话实时备份 |
| 是否用 LLM | ✅ 生成 slug | ❌ 纯机械导出 |

### 10.3 Echoryn 的 Hook 体系

Echoryn 的 Hook 系统基于 Plugin Framework，提供了标准化的生命周期钩子：

```go
// Echoryn (Go) — plugin framework 定义的 Hook 接口
type HookBeforeAgentStart interface {
    Plugin
    OnBeforeAgentStart(ctx context.Context, ev HookEvent) error
}

type HookAgentEnd interface {
    Plugin
    OnAgentEnd(ctx context.Context, ev HookEvent) error
}

// memory-core 插件实现了两个 Hook：
func (p *memoryCorePlugin) OnBeforeAgentStart(ctx context.Context, ev HookEvent) error {
    // 1. 触发增量同步
    p.mgr.Sync(ctx, manager.SyncOpts{Reason: "session-start"})
    
    // 2. 如果 PromptPipeline 未激活，走 Legacy 注入路径
    if !p.promptPipelineActive {
        results, _ := p.mgr.Search(ctx, ev.UserMessage())
        if len(results) > 0 {
            ev.AppendSystemMessage(formatMemoryContext(results))
        }
    }
    return nil
}

func (p *memoryCorePlugin) OnAgentEnd(ctx context.Context, ev HookEvent) error {
    // 轻量 Flush：截取最后一轮消息写入日期文件
    messages := ev.SessionMessages()
    lastUser, lastAssistant := extractLastRound(messages)
    entry := formatEntry(lastUser, lastAssistant)
    appendToFile(memoryDateFile(), entry)
    return nil
}
```

**Echoryn 的 Hook 目前相对简化**——`OnBeforeAgentStart` 负责同步 + 注入，`OnAgentEnd` 负责轻量 Flush。尚未实现 OpenClaw 那样的 LLM 驱动 Session Memory Hook。

### 10.4 Hook 系统对比

| 功能 | OpenClaw (TS) | Echoryn (Go) |
|------|:---:|:---:|
| before_agent_start | ✅ 同步 + 注入 | ✅ 同步 + 注入 |
| agent_end | ✅ | ✅ 轻量 Flush |
| session_before_compact | ✅ Compaction Safeguard | ❌ |
| command:new / command:reset | ✅ Session Memory Hook | ❌ |
| Session Transcript | ✅ 5s 去抖实时导出 | ❌ |
| Hook 注册方式 | 配置文件 + bundled | Go interface 编译时断言 |
| LLM 驱动 | ✅ slug 生成 | ❌ 纯机械 |

---

## 十一、Prompt 注入：让 Agent "想起"相关记忆

记忆搜索到结果后，需要以合适的方式注入 Agent 的 System Prompt。Echoryn 实现了**双路径注入**机制：

### 路径一：Prompt Pipeline Section（优先）

```go
// Echoryn (Go) — plugin.go
type MemorySection struct {
    plugin *memoryCorePlugin
}

func (s MemorySection) Priority() int { return 400 }

func (s MemorySection) Build(ctx context.Context, req prompt.BuildRequest) (string, error) {
    s.plugin.promptPipelineActive = true  // 标记 Pipeline 路径已激活
    
    results, err := s.plugin.mgr.Search(ctx, req.UserMessage,
        manager.WithMaxResults(s.plugin.cfg.Query.MaxResults),
        manager.WithMinScore(s.plugin.cfg.Query.MinScore))
    
    if len(results) == 0 { return "", nil }
    
    var buf strings.Builder
    buf.WriteString("## Relevant Memories\n\n")
    for _, r := range results {
        fmt.Fprintf(&buf, "### %s (lines %d-%d, score: %.2f)\n```\n%s\n```\n\n",
            r.Path, r.StartLine, r.EndLine, r.Score, r.Snippet)
    }
    return buf.String(), nil
}
```

### 路径二：Hook Legacy 路径（兜底）

```go
func (p *memoryCorePlugin) onBeforeAgentStart(ctx context.Context, ev plugin.HookEvent) error {
    // 如果 PromptPipeline 已经在工作，跳过 Hook 注入避免重复
    if p.promptPipelineActive {
        return p.mgr.Sync(ctx, manager.SyncOpts{Reason: "session-start"})
    }
    
    // Legacy 路径：手动搜索 + 注入到 System Prompt
    results, _ := p.mgr.Search(ctx, userMessage)
    if len(results) > 0 {
        ev.AppendSystemMessage(formatMemoryContext(results))
    }
    return nil
}
```

这个双路径设计保证了：
- 有 PromptPipeline 的新架构：通过 Section 注入，优先级可控（P:400）
- 无 PromptPipeline 的兼容场景：通过 Hook 注入，功能不受影响

### 11.1 自动注入 vs 工具调用：两种记忆召回策略

上面展示的 `MemorySection.Build()` 是一种**理想化的自动注入**方案——每轮对话都拿用户消息去做向量搜索，命中就注入。但实际上两个项目**真正运行的是另一种更轻量的策略**：**只注入指令，让 LLM 自己决定是否搜索**。

```
┌───────────────────────────────────────────────────────────┐
│ 策略 A：自动注入（理想方案）                                  │
│                                                           │
│ 每轮对话 → embedding(用户消息) → 向量搜索                    │
│        → 命中结果直接注入 System Prompt                      │
│                                                           │
│ 优点：Agent 不需要主动调用工具，结果自动出现在上下文中           │
│ 缺点：每轮都有 embedding + 搜索开销，即使问 "1+1=?" 也搜索    │
├───────────────────────────────────────────────────────────┤
│ 策略 B：工具调用（实际方案）  ← 两个项目都选择了这条路          │
│                                                           │
│ 每轮对话 → 只在 System Prompt 注入一段"指导说明"              │
│        → LLM 判断是否需要回忆 → 需要则调用 memory_search     │
│                                                           │
│ 优点：零额外开销（大部分对话不需要记忆），LLM 按需调用          │
│ 缺点：依赖 LLM 的判断力，可能漏搜或多搜                       │
└───────────────────────────────────────────────────────────┘
```

**Echoryn 的实际实现**——`MemorySection.Render()` 注入的是指导说明而非搜索结果：

```go
// Echoryn (Go) — 实际运行的代码
func (s MemorySection) Render() string {
    status := s.plugin.mgr.Status()
    if status.ChunkCount == 0 {
        return ""  // 没有索引内容，不注入任何指令
    }
    return fmt.Sprintf(`## Memory System
You have access to a persistent memory system with %d indexed files and %d content chunks.
- Before answering questions about past conversations, user preferences, or previous decisions, use the **memory_search** tool to find relevant context.
- When you learn important facts, preferences, or decisions, use the **memory_write** tool to save them.`,
        status.FileCount, status.ChunkCount)
}
```

门控条件只有一个：`ChunkCount > 0`（有索引内容才注入指令）。没有 embedding 计算，没有向量搜索。

**OpenClaw 的实际实现**——`buildMemorySection()` 同样只注入指令：

```typescript
// OpenClaw (TS) — 实际运行的代码
function buildMemorySection(): string {
    // 条件：非 subagent + 有 memory 工具可用
    return `## Memory Recall
Before answering anything about prior work, decisions, dates, preferences:
run memory_search on MEMORY.md + memory/*.md

Before recalling tasks, user context, or project-specific facts:
run memory_search with relevant keywords.`;
}
```

### 11.2 LLM 什么时候会搜索记忆？

答案是：**LLM 自己判断**。它收到上述指令后，根据用户问题的语义来决策：

| 用户消息 | LLM 判断 | 是否调用 memory_search |
|----------|----------|:---:|
| "我们上次讨论的架构方案是什么？" | 需要回忆过去的对话 | ✅ 搜索 |
| "我之前说过喜欢用什么框架来着？" | 需要回忆用户偏好 | ✅ 搜索 |
| "帮我写一个快速排序" | 全新任务，无需记忆 | ❌ 不搜索 |
| "1+1=?" | 简单问题 | ❌ 不搜索 |
| "继续之前的重构工作" | 需要回忆上次进度 | ✅ 搜索 |

### 11.3 为什么选择策略 B？

这个设计决策背后是**成本与收益的权衡**：

```
策略 A 的成本（每轮自动搜索）：
  - 1 次 embedding API 调用（~200ms + 费用）
  - 1 次向量搜索（~50ms）
  - 搜索结果注入 → 额外占用 context window（~500-2000 tokens）
  - 99% 的对话轮次不需要这些结果

策略 B 的成本（LLM 自主调用）：
  - 指令注入 ~100 tokens（固定）
  - 仅在需要时才有 embedding + 搜索开销
  - LLM 可能偶尔"忘记"调用 → 通过措辞优化 prompt 来改善
```

**实际观察**：在编程助手场景下，大约只有 5-10% 的对话轮次真正需要记忆搜索（用户问起过去的事情）。自动搜索意味着 90% 以上的搜索是浪费的。

不过，这两种策略**并非互斥**。代码中保留了策略 A 的完整实现（`MemorySection.Build()` 中带搜索的版本），未来可以根据场景切换：
- **编程助手**：策略 B（大部分对话是当前任务，不需要记忆）
- **个人助理**：策略 A 可能更合适（频繁需要回忆用户偏好和历史）

---

## 十二、对话压缩：上下文窗口的三层防线

长期记忆解决的是"跨会话"的信息持久化问题，但在**单次会话内**，随着多轮对话和工具调用不断累积，上下文也会快速膨胀直至触及模型的窗口上限。**对话压缩（Compaction）** 和 **上下文裁剪（Pruning）** 就是为了解决这个问题。

两个项目都实现了**三层防线**，从轻量到重量级依次部署：

```
用户消息进入
    │
    ▼
[第 1 层] Context Pruning — 内存级裁剪
    │  不改变持久化数据，只截断发送给 LLM 的副本
    │  目标：减少 tool result 的冗余内容
    ▼
[第 2 层] Reactive Compaction — 反应式压缩
    │  LLM 返回 context_overflow 错误时触发
    │  使用 LLM 对旧历史进行摘要压缩
    ▼
[第 3 层] Proactive Compaction — 主动式压缩
    │  每轮成功后检查 token 阈值
    │  提前压缩，防止下一轮溢出
    ▼
正常继续对话
```

### 12.1 Token 估算

压缩的前提是知道当前消耗了多少 token。但本地运行 tokenizer 太重（tiktoken 是 Python 库），两个项目都采用**启发式估算**：

```go
// Echoryn (Go) — token_estimator.go
const defaultCharsPerToken = 3.5  // 英文 ~4, CJK ~2 的折中值

func (e *TokenEstimator) EstimateString(s string) int {
    return int(math.Ceil(float64(len([]rune(s))) / e.charsPerToken))
}

func (e *TokenEstimator) EstimateMessage(m *Message) int {
    tokens := e.EstimateString(m.Content)
    tokens += 4  // role token + 分隔符开销
    for _, tc := range m.ToolCalls {
        tokens += e.EstimateString(tc.Function.Name)
        tokens += e.EstimateString(tc.Function.Arguments)
        tokens += 4  // 工具调用框架开销
    }
    return tokens
}
```

```typescript
// OpenClaw (TS) — compaction.ts
function estimateMessagesTokens(messages: Message[]): number {
    return messages.reduce((sum, m) => {
        return sum + Math.ceil((m.content?.length ?? 0) / 4) + 4;
    }, 0);
}
```

误差通常在 10-20%，对于"是否需要压缩"的判断来说足够了。

### 12.2 上下文窗口守卫 (Context Window Guard)

在压缩之前，首先需要知道模型的上下文窗口到底有多大。两个项目都实现了**多级回退解析**：

```
解析优先级：模型元数据 → 配置覆盖 → 硬编码回退
```

```go
// Echoryn (Go) — context_window.go
const (
    HardMinimumContextWindow = 16_000    // 低于此值拒绝执行
    WarnContextWindow        = 32_000    // 低于此值打 warning
    DefaultContextWindow     = 200_000   // Claude Opus 级别的默认值
)

func (g *ContextWindowGuard) Resolve(model string) ContextWindowInfo {
    windowSize := g.lookupModelWindow(model)  // 查模型元数据
    if windowSize == 0 {
        windowSize = g.config.DefaultWindow   // 配置默认值
    }
    if windowSize == 0 {
        windowSize = DefaultContextWindow     // 硬编码回退
    }
    reserveTokens := min(g.config.ReserveTokens, windowSize/2)
    return ContextWindowInfo{
        WindowSize:    windowSize,
        ReserveTokens: reserveTokens,
        UsableTokens:  windowSize - reserveTokens,
    }
}
```

`UsableTokens` = 窗口总大小 - 输出保留量（`ReserveTokens` 不超过窗口的 50%），这才是可用于输入的 token 预算。

### 12.3 第一层：Context Pruning（内存级裁剪）

这是最轻量的防线——**不改变任何持久化数据**，只在发送给 LLM 之前，对消息副本进行内存级裁剪。

两个项目的策略高度一致，都采用**两阶段裁剪**：

```
┌─────────────────────────────────────────────────┐
│              Context Pruner                       │
│                                                  │
│  ratio = estimatedTokens / usableTokens          │
│                                                  │
│  Stage 1: Soft-Trim (ratio > 0.3)               │
│  ┌──────────────────────────────────┐            │
│  │ 旧的 tool result 消息:           │            │
│  │ "很长的工具输出结果..."           │            │
│  │         ↓ 截断为                 │            │
│  │ head(1500 chars)                 │            │
│  │ ...[N characters truncated]...   │            │
│  │ tail(1500 chars)                 │            │
│  └──────────────────────────────────┘            │
│                                                  │
│  Stage 2: Hard-Clear (ratio > 0.5)              │
│  ┌──────────────────────────────────┐            │
│  │ 更旧的 tool result 消息:         │            │
│  │         ↓ 替换为                 │            │
│  │ "[Old tool result cleared]"      │            │
│  └──────────────────────────────────┘            │
│                                                  │
│  保护: 最近 3 条 assistant 之后的内容不裁剪       │
└─────────────────────────────────────────────────┘
```

```go
// Echoryn (Go) — context_pruner.go
func (p *ContextPruner) Prune(messages []Message, usable int) []Message {
    estimated := p.estimator.EstimateMessages(messages)
    ratio := float64(estimated) / float64(usable)

    // 深拷贝，不修改原始数据
    pruned := deepCopyMessages(messages)

    // 找到保护边界：最近 3 条 assistant 消息之后的所有内容不动
    protectedFrom := findProtectedBoundary(pruned, p.keepRecent)

    if ratio > 0.3 {
        softTrimToolResults(pruned[:protectedFrom], 1500, 1500)
    }
    if ratio > 0.5 {
        hardClearToolResults(pruned[:protectedFrom])
    }
    return pruned
}
```

OpenClaw 的实现在此基础上多了三个维度的精细控制：

**TTL 缓存模式**——不是每次构建上下文都重新裁剪，而是设置一个 5 分钟的 TTL。在 TTL 窗口内，直接使用上次裁剪的结果，减少重复计算：

```typescript
// OpenClaw — context-pruning/extension.ts
class ContextPruningExtension {
    private lastCacheTouchAt = 0;
    private readonly cacheTtlMs = 5 * 60 * 1000;  // 5 分钟
    
    shouldPrune(): boolean {
        const now = Date.now();
        if (now - this.lastCacheTouchAt < this.cacheTtlMs) {
            return false;  // TTL 内跳过裁剪
        }
        this.lastCacheTouchAt = now;
        return true;
    }
}
```

**工具白名单/黑名单**——通过 glob 模式控制哪些工具的结果可以被裁剪：

```typescript
// OpenClaw — context-pruning/settings.ts
interface PruningSettings {
    tools: {
        allow?: string[];  // 只裁剪这些工具的结果（glob）
        deny?: string[];   // 不裁剪这些工具的结果（glob）
    };
    // 例如: deny: ["Read*", "search_*"] — 文件读取和搜索结果不裁剪
    // 例如: allow: ["bash*"] — 只裁剪 bash 工具的输出
    
    minPrunableToolChars: 50_000;  // 低于 50K chars 的结果不做 hard-clear
}
```

**图片块处理**——含图片的工具结果永远不裁剪（但按 8K chars 计入 token 预算）。

### 12.4 第二层：Reactive Compaction（反应式压缩）

当 Pruning 不够用，LLM API 返回上下文溢出错误时，触发**反应式压缩**——使用 LLM 对旧历史生成摘要，然后重试。

**溢出错误检测**（两个项目检测相同的错误模式）：

```go
// Echoryn (Go) — executor.go
func isContextOverflowError(err error) bool {
    patterns := []string{
        "context_length_exceeded",
        "maximum context length",
        "too many tokens",
        "request_too_large",
        "exceeds model context window",
        "413 request entity too large",
    }
    msg := strings.ToLower(err.Error())
    for _, p := range patterns {
        if strings.Contains(msg, p) { return true }
    }
    return false
}
```

**反应式压缩流程**：

```
LLM 返回 context_overflow
    │
    ▼
是否已尝试过压缩? ──yes──→ 放弃，返回错误
    │ no
    ▼
获取一个可用的 ChatModel
    │
    ▼
Compactor.Compact()
    │
    ▼
重建上下文 (ContextBuilder.Build)
    │
    ▼
重试 LLM 调用
```

### 12.5 第三层：Proactive Compaction（主动式压缩）

最重要的一层——**不等到溢出才补救，每轮成功后主动检查**：

```go
// Echoryn (Go) — runner.go
func (r *AgentRunner) checkProactiveCompaction(ctx context.Context, session *Session) {
    estimated := r.estimator.EstimateMessages(session.ActiveMessages())
    ratio := float64(estimated) / float64(r.ctxWindow.UsableTokens)

    if ratio > r.config.CompactionThreshold {  // 默认 0.8
        log.Info("Proactive compaction triggered", "ratio", ratio)
        r.compactor.Compact(ctx, session)
        r.sessionRepo.Save(session)
    }
}
```

这意味着当对话累计使用了 80% 的可用窗口时，**在用户下次发消息之前**就完成压缩。

### 12.6 压缩器核心算法 (Compactor)

压缩器是三层防线中最重的组件——它调用 LLM 将旧对话历史浓缩为一段摘要。

**分割策略**：保留最近 N 轮（默认 3 个 user→assistant 交互），其余被压缩。

```
Session 消息列表:
[msg0, msg1, ..., msg15, msg16, msg17, msg18, msg19, msg20]
                   ↑                                  ↑
              分割点 (保留最近 3 轮)              最新消息

待摘要: [msg0 ... msg15]
保  留: [msg16 ... msg20]  ← 这些不动
```

**分块摘要**（当待摘要内容太长时）：

```go
// Echoryn (Go) — compaction.go
func (c *Compactor) Compact(ctx context.Context, session *Session) error {
    // 1. 找分割点：保留最近 N 个 user→assistant 轮对
    splitIdx := findSplitPoint(session.ActiveMessages(), c.keepRecentTurns)

    // 2. 将待摘要消息按 token 预算分块（每块 ≤ 窗口 40%）
    toSummarize := session.ActiveMessages()[:splitIdx]
    chunks := chunkByTokenBudget(toSummarize, c.windowInfo.UsableTokens * 40 / 100)

    // 3. 对每块调用 LLM 生成摘要
    partialSummaries := make([]string, len(chunks))
    for i, chunk := range chunks {
        summary, err := c.summarizeChunk(ctx, chunk, session.CompactionSummary)
        if err != nil {
            summary = fmt.Sprintf("[Summary of %d messages unavailable]", len(chunk))
        }
        partialSummaries[i] = summary
    }

    // 4. 如果多块，合并部分摘要为最终摘要
    finalSummary := c.mergeSummaries(ctx, partialSummaries)

    // 5. 更新 Session 状态
    session.ApplyCompaction(finalSummary, absoluteKeptFrom)
    return nil
}
```

**摘要 Prompt** 要求 LLM 保留的信息类型：

```
请将以下对话历史浓缩为简洁的摘要。保留:
- 关键决策和结论
- 重要事实和数据点
- 仍然相关的工具调用结果（文件路径、配置值等）
- 用户偏好和需求

摘要预算: 不超过 {budget} tokens
```

**容错设计**：
- 单个 chunk 摘要失败 → 使用 `"[Summary of N messages unavailable]"` 占位
- 合并摘要失败 → 直接拼接部分摘要
- 超长消息（>2000 字符）→ 在 Prompt 中截断为 `head(1000) + tail(500)`

### 12.7 Session 的压缩状态持久化

```go
// Echoryn (Go) — session.go
type Session struct {
    Messages          []*Message
    CompactionSummary string  // LLM 生成的摘要
    CompactionCount   int     // 压缩次数
    FirstKeptIndex    int     // 从哪个索引开始保留原始消息
}

// 活跃消息 = 压缩点之后的消息
func (s *Session) ActiveMessages() []*Message {
    return s.Messages[s.FirstKeptIndex:]
}

// 应用一次压缩
func (s *Session) ApplyCompaction(summary string, keptFrom int) {
    s.CompactionSummary = summary
    s.FirstKeptIndex = keptFrom
    s.CompactionCount++
}

func (s *Session) HasCompaction() bool {
    return s.CompactionSummary != ""
}
```

**关键设计**：原始消息**不删除**，只是通过 `FirstKeptIndex` 标记"活跃窗口"。这意味着：
- 压缩前的历史仍然保留在磁盘上（可审计）
- `ActiveMessages()` 只返回活跃部分（发给 LLM）
- `CompactionSummary` 作为独立的 system message 注入上下文

### 12.8 上下文构建的完整组装

所有组件最终在 `ContextBuilder` 中汇合：

```go
// Echoryn (Go) — context_builder.go
func (b *ContextBuilder) Build(ctx context.Context, session *Session, userInput string) []Message {
    var messages []Message

    // Layer 1: System Prompt
    systemPrompt := b.promptPipeline.Build(ctx)
    messages = append(messages, SystemMessage(systemPrompt))

    // Layer 2: Compaction Summary (如果有)
    if session.HasCompaction() {
        messages = append(messages, SystemMessage(
            "[Conversation Summary]\n" + session.CompactionSummary,
        ))
    }

    // Layer 3: Memory-injected messages (来自插件)
    messages = append(messages, b.injectedMessages...)

    // Layer 4: Session History (仅活跃消息，受轮次限制)
    active := session.ActiveMessages()
    limited := b.limitHistoryTurns(active, b.maxHistoryTurns)  // 默认保留 50 轮
    messages = append(messages, limited...)

    // Layer 5: Current User Input
    messages = append(messages, UserMessage(userInput))

    // Final: Context Pruning
    return b.pruner.Prune(messages, b.windowInfo.UsableTokens)
}
```

**注入顺序很重要**：
1. System Prompt — LLM 的"人格"和指令
2. 压缩摘要 — 被压缩掉的历史的浓缩版
3. 记忆注入 — 长期记忆的相关片段
4. 活跃历史 — 未被压缩的近期对话
5. 当前输入 — 用户这次说了什么

### 12.9 OpenClaw 的额外特性：生产级压缩护栏

OpenClaw 的压缩系统比 Echoryn 多了四个重要的生产级特性。这些特性解决的都是**真实生产环境中遇到的问题**——当对话足够长、工具调用足够多时，简单的摘要压缩会丢失关键上下文。

#### 12.9.1 Compaction Safeguard（压缩安全护栏）

Compaction Safeguard 注册在 `session_before_compact` 事件上，在 LLM 执行摘要之前，预处理待压缩的消息，确保关键信息不会被丢失。

```typescript
// OpenClaw — compaction-safeguard.ts
// 五大保护机制

// 1. 文件操作跟踪 — 区分 read/edited/written 文件
function computeFileLists(messages: Message[]): FileTrackingResult {
    const readFiles = new Set<string>();
    const editedFiles = new Set<string>();
    // 扫描所有 tool_use 块，提取文件路径
    for (const msg of messages) {
        for (const block of msg.content) {
            if (block.type === 'tool_use' && block.name === 'Read') {
                readFiles.add(block.input.file_path);
            }
            if (block.name === 'Edit' || block.name === 'Write') {
                editedFiles.add(block.input.file_path);
            }
        }
    }
    return { readFiles, editedFiles };
}

// 2. 工具失败收集 — 最多 8 个，每个 max 240 chars
function collectToolFailures(messages: Message[]): string[] {
    const failures: string[] = [];
    for (const msg of messages) {
        if (msg.role === 'tool' && isErrorResult(msg)) {
            failures.push(truncate(msg.content, 240));
            if (failures.length >= 8) break;
        }
    }
    return failures;
}

// 3. Workspace 上下文注入 — 从 AGENTS.md 提取关键节
function injectWorkspaceContext(): string {
    const agentsMd = readFile('AGENTS.md');
    const sessionStartup = extractSection(agentsMd, 'Session Startup');
    const redLines = extractSection(agentsMd, 'Red Lines');
    return truncate(sessionStartup + '\n' + redLines, 2000);
    // 限 2000 chars，避免注入内容本身导致溢出
}

// 4. 自适应分块比例
function computeAdaptiveChunkRatio(messages: Message[]): number {
    const BASE = 0.4;    // 基础比例：窗口的 40%
    const MIN = 0.15;    // 最低比例：窗口的 15%
    const avgMsgSize = totalChars(messages) / messages.length;
    // 消息越大 → 分块越小（避免单块超出 LLM 处理能力）
    return Math.max(MIN, BASE * (1 - avgMsgSize / MAX_MSG_SIZE));
}

// 5. 分裂 turn 处理 — 当压缩切点在一个 turn 中间时
// 额外为 turn 前缀生成摘要，避免丢失上文
```

**为什么需要文件跟踪？** 压缩摘要通常会丢失具体的文件路径。但如果 Agent 正在一个长任务中编辑多个文件，压缩后它不知道哪些文件已经读过、哪些已经改过。文件列表注入摘要后，Agent 可以继续工作而不需要重新读取所有文件。

#### 12.9.2 Post-Compaction Recovery（压缩后恢复）

压缩完成后，OpenClaw 不是简单地继续对话，而是注入一个**恢复 prompt**，让 Agent 重新执行启动序列：

```typescript
// OpenClaw — post-compaction-context.ts
function buildPostCompactionContext(): string {
    const agentsMd = readFile('AGENTS.md');
    
    // 提取关键节（代码块感知解析，不会切断代码块）
    const startup = extractSection(agentsMd, 'Session Startup');
    const redLines = extractSection(agentsMd, 'Red Lines');
    
    const context = truncate(startup + '\n' + redLines, 3000);
    
    return `[Post-compaction context refresh]
${context}

Execute your Session Startup sequence now — 
read the required files before responding to the user.`;
}

// extractSection() — H2/H3 级别匹配，跳过代码块
function extractSection(md: string, heading: string): string {
    const lines = md.split('\n');
    let capturing = false;
    let depth = 0;
    let inCodeBlock = false;
    const result: string[] = [];
    
    for (const line of lines) {
        if (line.startsWith('```')) inCodeBlock = !inCodeBlock;
        if (inCodeBlock) { if (capturing) result.push(line); continue; }
        
        if (line.match(/^#{2,3}\s/) && line.includes(heading)) {
            capturing = true;
            depth = line.startsWith('###') ? 3 : 2;
            continue;
        }
        if (capturing && line.match(/^#{2,3}\s/) && !line.includes(heading)) {
            break;  // 遇到同级或更高级标题，停止
        }
        if (capturing) result.push(line);
    }
    return result.join('\n');
}
```

这个设计的精妙之处在于：压缩后 Agent 会"失忆"——它只有一段摘要和最近几轮对话。恢复 prompt 提醒它重新读取必要的文件（比如项目的 README、AGENTS.md），重建工作上下文。

#### 12.9.3 Tool Result Truncation（工具结果截断）

独立于 Context Pruner 的**Session 级截断**机制——在工具返回结果时立即截断，而不是等到构建上下文时：

```typescript
// OpenClaw — tool-result-truncation.ts
const MAX_TOOL_RESULT_CONTEXT_SHARE = 0.3;   // 单个结果最多占窗口 30%
const HARD_MAX_TOOL_RESULT_CHARS = 400_000;   // 绝对硬上限 400K chars
const MIN_KEEP_CHARS = 2_000;                  // 最少保留 2K chars

function truncateToolResult(result: string, contextWindow: number): string {
    const maxChars = Math.min(
        contextWindow * 4 * MAX_TOOL_RESULT_CONTEXT_SHARE,
        HARD_MAX_TOOL_RESULT_CHARS
    );
    const keepChars = Math.max(maxChars, MIN_KEEP_CHARS);
    
    if (result.length <= keepChars) return result;
    
    // 在行边界处截断（不会切断一行的中间）
    const breakPoint = result.lastIndexOf('\n', keepChars);
    return result.substring(0, breakPoint > 0 ? breakPoint : keepChars)
        + `\n\n[... ${result.length - keepChars} characters truncated ...]`;
}
```

**为什么还需要 Context Guard？** Tool Result Truncation 在工具返回时截断，但有时多个工具调用的**累计**结果仍然超出上下文预算。Context Guard 是第二道防线：

```typescript
// OpenClaw — tool-result-context-guard.ts
// monkey-patch transformContext，在发送 LLM 前最后检查

const CONTEXT_INPUT_HEADROOM_RATIO = 0.75;     // 总预算：窗口的 75%
const SINGLE_TOOL_RESULT_CONTEXT_SHARE = 0.5;  // 单个结果最多 50%

function enforceContextBudget(messages: Message[], budget: number): Message[] {
    // Phase 1: 单个 tool result 不超过 50%
    for (const msg of messages) {
        if (msg.role === 'tool' && charCount(msg) > budget * 0.5) {
            msg.content = '[compacted: tool output removed to free context]';
        }
    }
    
    // Phase 2: 总量不超过 75%，从最旧的 tool result 开始清除
    let total = sumChars(messages);
    for (let i = 0; i < messages.length && total > budget; i++) {
        if (messages[i].role === 'tool') {
            total -= charCount(messages[i]);
            messages[i].content = '[compacted: tool output removed to free context]';
        }
    }
    return messages;
}
```

#### 12.9.4 `/compact` 命令

用户可以在 TUI 中手动触发压缩，而不只是等系统自动触发：

```typescript
// OpenClaw — commands-compact.ts
// 用户输入 /compact → 立即执行压缩流水线
// 完整流程: Memory Flush → Compaction → Post-Compaction Recovery
```

**四大额外特性的协作关系**：

```
对话进行中...
    │
    ├─ 每次工具返回 → Tool Result Truncation (30% cap)
    │
    ├─ 每次发送 LLM → Context Guard (75% total budget)
    │
    ├─ 接近压缩阈值 → Memory Flush (静默 turn)
    │
    ├─ 触发压缩 → Compaction Safeguard
    │     ├─ 文件跟踪
    │     ├─ 工具失败收集
    │     ├─ Workspace 上下文注入
    │     └─ 自适应分块
    │
    └─ 压缩完成 → Post-Compaction Recovery
          └─ 注入恢复 prompt + 重执行启动序列
```

### 12.10 三层防线对比

| 维度 | Context Pruning | Reactive Compaction | Proactive Compaction |
|------|:---:|:---:|:---:|
| 触发时机 | 每次构建上下文 | LLM 返回溢出错误 | 每轮成功后检查 |
| 改变持久化数据 | ❌ 仅内存副本 | ✅ 更新 Session | ✅ 更新 Session |
| 使用 LLM | ❌ | ✅ 需要额外 API 调用 | ✅ 需要额外 API 调用 |
| 效果 | 截断/清除工具结果 | 旧历史 → 摘要 | 旧历史 → 摘要 |
| 性能代价 | 几乎为零 | 1-2 次 LLM 调用 | 1-2 次 LLM 调用 |
| 信息损失 | 工具结果细节 | 旧对话细节 | 旧对话细节 |

---

## 十三、Memory Flush：压缩前的记忆沉淀

Memory Flush 不是简单的"对话结束后保存"，而是一个与 Compaction 流水线深度耦合的**预压缩持久化**机制。当对话即将触发压缩时，先让 Agent 把重要信息写入长期记忆，这样即使压缩后丢失了历史细节，长期记忆中仍有备份。

### 13.1 OpenClaw：Pre-Compaction LLM 驱动 Flush

OpenClaw 的 Memory Flush 是一个精密的阈值检测 + 静默 turn 机制：

```typescript
// OpenClaw — memory-flush.ts
// 核心触发条件：token 用量接近上下文极限

function shouldRunMemoryFlush(
    totalTokens: number,
    contextWindow: number,
    reserveFloor: number,
    softThreshold: number,   // 默认 4000 tokens
    compactionCount: number,
    lastFlushCompactionCount: number
): boolean {
    // 条件 1：每次 compaction 周期最多 flush 一次
    if (compactionCount <= lastFlushCompactionCount) return false;
    
    // 条件 2：token 用量接近极限
    // totalTokens >= contextWindow - reserveFloor - softThreshold
    const threshold = contextWindow - reserveFloor - softThreshold;
    return totalTokens >= threshold;
}
```

**触发后的执行流程：**

```
Token 用量检测
    │
    ▼
shouldRunMemoryFlush() === true
    │
    ▼
发起一个"静默 turn"（Silent Turn）
    │
    ├─ System Prompt: DEFAULT_MEMORY_FLUSH_PROMPT
    │   "你即将失去对话历史。请将重要的用户偏好、
    │    项目上下文、关键决策写入记忆文件。"
    │
    ├─ Agent 自主决定调用 memory_write / memory_store
    │
    ├─ Agent 回复带有 <silent> 标记
    │   → 不显示给用户（静默执行）
    │
    └─ 更新 lastFlushCompactionCount = compactionCount
         → 防止同一 compaction 周期重复 flush
```

**为什么叫"静默 turn"？** 因为这是一个对用户完全透明的 Agent 执行——用户看不到任何输出。Agent 收到特殊的 flush prompt 后，自行分析对话历史，提取值得持久化的信息，调用记忆工具写入，然后回复一个 `<silent>` 标记。整个过程在用户无感知的情况下完成。

**关键配置项：**

```typescript
interface MemoryFlushConfig {
    enabled: boolean;                  // 是否启用
    softThresholdTokens: number;       // 软阈值，默认 4000
    prompt: string;                    // flush 提示词
    systemPrompt: string;              // 系统提示词覆盖
    reserveTokensFloor: number;        // 输出保留 token 底线
}
```

### 13.2 Echoryn：轻量直接 Flush

Echoryn 目前采用更简单的策略——在 `OnAgentEnd` Hook 中机械截取最后一轮消息写入记忆文件：

```go
// Echoryn — plugin.go
func (p *memoryCorePlugin) onAgentEnd(ctx context.Context, ev plugin.HookEvent) error {
    messages := ev.SessionMessages()
    lastUser, lastAssistant := extractLastRound(messages)
    
    entry := fmt.Sprintf("## %s\n\n**User**: %s\n\n**Assistant**: %s\n",
        time.Now().Format("15:04:05"), lastUser, lastAssistant)
    
    filePath := filepath.Join("memory", time.Now().Format("2006-01-02") + ".md")
    appendToFile(filePath, entry)  // append-only，不覆盖
    return nil
}
```

### 13.3 两种 Flush 策略的深层对比

| 维度 | OpenClaw (Pre-Compaction Flush) | Echoryn (Direct Flush) |
|------|------|------|
| **触发时机** | 压缩前（token 接近极限） | 每轮对话结束后 |
| **触发条件** | `totalTokens >= contextWindow - reserve - 4K` | 无条件 |
| **频率控制** | 每 compaction 周期最多 1 次 | 每轮 1 次 |
| **记忆质量** | 高（LLM 筛选要点） | 中（原始对话截取） |
| **API 成本** | 额外 1 次 LLM 调用 | 零额外成本 |
| **用户感知** | 无（`<silent>` 静默执行） | 无（后台写入） |
| **写入格式** | LLM 自主决定 | 固定 `## HH:MM:SS` + User/Assistant |
| **文件策略** | LLM 决定写入路径 | 固定 `memory/YYYY-MM-DD.md`，append-only |
| **与压缩协同** | ✅ 深度耦合（压缩前保存） | ❌ 独立（不感知压缩状态） |

OpenClaw 的 Pre-Compaction Flush 真正巧妙之处在于**时机选择**：在压缩即将发生时才 flush，既避免了每轮都调用 LLM 的浪费，又确保了即将被压缩掉的细节有机会被持久化。这是一种"最后一刻的抢救"策略。

---

## 十四、OpenClaw vs Echoryn 对比总结

### 功能矩阵

| 功能 | OpenClaw (TS) | Echoryn (Go) |
|------|:---:|:---:|
| **搜索核心** | | |
| 向量搜索: vec0 KNN | ✅ | ✅ |
| 向量搜索: brute-force 回退 | ✅ | ✅ |
| 关键词搜索: FTS5 | ✅ | ✅ |
| 混合搜索权重 | 0.7 / 0.3 | 0.7 / 0.3 |
| MMR 重排序 | ✅ (Jaccard, λ=0.7) | ✅ (Jaccard, λ=0.7) |
| 时间衰减 | ✅ (半衰期 30 天) | ✅ (半衰期 30 天) |
| 查询扩展 (7 语言) | ✅ | ✅ |
| **Embedding** | | |
| OpenAI | ✅ | ✅ |
| Gemini | ✅ | ✅ |
| 本地 (Ollama/GGUF) | ✅ | ❌ |
| Embedding 缓存 | ✅ | ✅ |
| **记忆工具** | | |
| memory_search | ✅ | ✅ |
| memory_get / memory_read | ✅ | ✅ |
| memory_write | ❌ | ✅ |
| memory_delete | ❌ | ✅ |
| **LanceDB 扩展工具** | | |
| memory_recall | ✅ | ❌ |
| memory_store | ✅ | ❌ |
| memory_forget | ✅ | ❌ |
| Auto-Recall (Hook 自动注入) | ✅ | ❌ |
| Auto-Capture (规则引擎+去重) | ✅ | ❌ |
| **索引与同步** | | |
| 增量同步 (SHA-256) | ✅ | ✅ |
| 文件监控 (fsnotify) | ✅ | ✅ |
| QMD 外部后端 | ✅ | ❌ |
| 远程/批量同步 | ✅ | ❌ |
| **Hook 系统** | | |
| before_agent_start | ✅ | ✅ |
| agent_end | ✅ | ✅ |
| Session Memory Hook | ✅ (LLM slug) | ❌ |
| session_before_compact | ✅ | ❌ |
| Session Transcript | ✅ (5s 去抖) | ❌ |
| **上下文管理** | | |
| Prompt Pipeline 集成 | 无（Hook only） | ✅ 双路径 |
| Context Pruning | ✅ 含 TTL + 白/黑名单 | ✅ 基础两阶段 |
| Tool Result Truncation | ✅ (30% cap + 400K max) | ❌ |
| Tool Result Context Guard | ✅ (75% total budget) | ❌ |
| Reactive Compaction | ✅ | ✅ |
| Proactive Compaction | ✅ | ✅ (阈值 0.8) |
| Compaction Safeguard | ✅ 5 大保护机制 | ❌ |
| Post-Compaction Recovery | ✅ | ❌ |
| Memory Flush | ✅ Pre-Compaction 静默 turn | ✅ 轻量直接 |
| 手动 /compact 命令 | ✅ | ❌ |
| 上下文窗口守卫 | ✅ | ✅ |

### 工程亮点对比

| 维度 | OpenClaw | Echoryn |
|------|----------|---------|
| 代码规模 | manager.ts 73KB，功能密集 | manager.go ~800 行，职责分层 |
| 并发模型 | JS 单线程 + async/await | Go goroutine + atomic CAS |
| 插件集成 | 独立模块，手动注册 | K8S Plugin Framework，编译时接口断言 |
| 类型安全 | TypeScript 泛型 | Go interface + type alias |
| 错误处理 | try/catch + optional chaining | 显式 error return + 优雅降级 |
| 记忆后端 | SQLite builtin + QMD + LanceDB | SQLite builtin only |
| 搜索管线 | 向量+FTS → 时间衰减 → MMR | 向量+FTS → 时间衰减 → MMR |
| Compaction 深度 | 5 层防护 (Pruning→Truncation→Guard→Safeguard→Recovery) | 3 层防线 (Pruning→Reactive→Proactive) |

### 设计哲学差异

两个项目虽然共享同一套设计理念，但在"完备性 vs 简洁性"上做出了不同选择：

- **OpenClaw** 追求**生产级完备性**——每一个边界情况都有对应的处理机制（TTL、白名单、分裂 turn、Post-Recovery 等）。代价是代码复杂度高，单个文件动辄 300+ 行。
- **Echoryn** 追求**架构简洁性**——用 Go 的 interface + Plugin Framework 实现清晰的关注点分离，先把 80% 的核心功能做到位，再逐步补齐。代价是部分高级特性（如 Compaction Safeguard）暂缺。

---

## 十五、后续演进方向

1. **智能 Memory Flush** — Echoryn 计划引入 LLM 驱动的 Pre-Compaction Flush，在 API 成本和记忆质量间找到更好的平衡
2. **Ollama Embedding** — 支持本地 Embedding 模型，满足离线 / 隐私敏感场景
3. **Session Memory Hook** — 实现会话切换时的 LLM 驱动记忆沉淀（slug 生成 + Markdown 导出）
4. **Compaction Safeguard** — 引入文件跟踪、工具失败收集、Post-Compaction Recovery 等生产级护栏
5. **Tool Result Truncation** — 实现 Session 级截断和 Context Guard 双层保护
6. **Context Pruning 增强** — 支持 TTL 缓存模式和工具白/黑名单 glob 过滤
7. **LanceDB 扩展** — 考虑引入向量数据库扩展，提供 Auto-Recall / Auto-Capture 能力
8. **记忆压缩与合并** — 长期运行后记忆文件会膨胀，需要定期合并和压缩策略

---

*本文基于 OpenClaw (TypeScript) 和 Echoryn (Go) 的实际代码撰写。两个项目共享设计理念但在实现细节上各有创新，展现了同一问题在不同语言生态中的工程解法。*
