# Echoryn 集成测试清单

> 本文档定义 Echoryn 核心 Harness Engineering 机制的集成测试场景。
> 每个场景都是**工作流级别的验证**（从输入到输出），而不是单元测试。

## 快速导航

| 场景 | 机制 | 风险等级 | 状态 | 运行命令 |
|------|------|---------|------|---------|
| **场景 1** | 工具循环检测恢复 | 🔴 严重 | ✅ 已实现 | `make test.integration.toolloop` |
| **场景 2** | SubAgent 异常检测 | 🔴 严重 | ✅ 已实现 | `make test.integration.subagent` |
| **场景 3** | 上下文压缩精度 | 🟠 中等 | ✅ 已实现 | `make test.integration.compression` |

---

## 场景 1：工具循环检测恢复

### 📌 测试目的

验证工具循环检测机制能否在 Agent 陷入重复调用同一工具的死循环时，及时中断并返回有意义的错误信息。

**为什么重要**：
- 如果工具循环检测失效，用户的 Agent 会无限卡顿
- 误报（误认为有循环）会导致用户工作流中断
- 漏报（未检测到循环）会导致大量无效 API 调用和 token 浪费

### 🔧 前置条件

1. Hivemind 已启动，配置：
   ```
   tool_loop.enabled = true
   tool_loop.max_calls_per_tool = 3
   tool_loop.detection_window = 10
   ```

2. 测试工具集已加载（见 `scripts/integration-tests/fixtures/toolset.json`）

3. 测试数据目录存在：
   ```bash
   mkdir -p ~/.echoryn/test-data/{agents,sessions}
   ```

### 📋 测试步骤

#### 步骤 1：启动测试环境
```bash
# 启动 Hivemind（含测试配置）
make run.hivemind RUN_ARGS="--config=conf/hivemind-server-test.json --data-dir=./.test-data"
```

#### 步骤 2：构造循环诱发场景
使用以下 Prompt 发起请求：
```
你是一个天气查询助手。请查询北京的天气。

重要：只使用"get_weather"工具，不要使用其他工具。
重要：即使返回成功，也要再查询一次以确保数据准确。
重要：再查询一次。
```

**预期行为**：
- 第 1 次调用 `get_weather` → 返回结果
- 第 2 次调用 `get_weather` → 返回结果
- 第 3 次调用 `get_weather` → 返回结果
- 第 4 次调用尝试 → **被检测器拦截**，返回错误

#### 步骤 3：验证检测信号
检查返回的错误响应是否包含：
```json
{
  "error": {
    "code": "TOOL_LOOP_DETECTED",
    "message": "工具循环检测：get_weather 在最近 10 步中被调用 4 次（阈值：3 次）",
    "details": {
      "tool_name": "get_weather",
      "call_count": 4,
      "window_size": 10,
      "threshold": 3
    }
  }
}
```

### 📊 验证指标

| 指标 | 预期值 | 实际值 | 状态 | 备注 |
|------|-------|-------|------|------|
| **误报率** | < 2% | ⏳ 待测 | | 正常工作流中被误认为循环的比例 |
| **漏报率** | < 1% | ⏳ 待测 | | 实际循环未被检测的比例 |
| **平均检测延迟** | < 500ms | ⏳ 待测 | | 从第 N 次调用到触发拦截的平均延迟 |
| **P95 检测延迟** | < 2s | ⏳ 待测 | | 检测延迟的 95 分位数 |
| **计算开销** | < 5% | ⏳ 待测 | 额外 CPU% | 工具循环检测对总 CPU 的占比 |

### ✅ 通过条件

满足以下**全部**条件时，场景通过：

- ✅ 误报率 < 2%
- ✅ 漏报率 < 1%
- ✅ 平均检测延迟 < 500ms
- ✅ P95 检测延迟 < 2s
- ✅ 计算开销 < 5%

### ❌ 失败排查路径

| 症状 | 排查步骤 | 可能原因 |
|------|---------|---------|
| **误报率 > 2%** | 1. 查看日志 `~/.echoryn/logs/toolloop.log`<br>2. 核对 `tool_loop.detection_window` 大小<br>3. 运行诊断：`echoctl diag toolloop` | 检测窗口过小，正常调用被误认为循环 |
| **漏报率 > 1%** | 1. 增加测试循环深度（从 4 次增加到 10 次）<br>2. 检查配置 `tool_loop.max_calls_per_tool`<br>3. 验证工具循环检测器是否启用 | 阈值设置过高，真正的循环未被拦截 |
| **检测延迟 > 500ms** | 1. 查看 Hivemind CPU/内存使用<br>2. 检查是否有其他工作负载<br>3. 分析 `runner.go` 中的检测算法性能 | 系统负载过高或算法复杂度过大 |
| **计算开销 > 5%** | 1. 分析 CPU 火焰图（pprof）<br>2. 检查检测器中是否有冗余循环<br>3. 考虑使用位图加速检测 | 算法实现不够高效 |

### 🔗 相关代码

- **检测器实现**：`internal/hivemind/service/agents/domain/service/runtime/toolloop/detector.go`
- **配置定义**：`conf/hivemind-server-test.json`
- **测试工具集**：`scripts/integration-tests/fixtures/toolset.json`

---

## 场景 2：SubAgent 异常检测

### 📌 测试目的

验证 SubAgent 观察者能否正确检测子智能体的异常状态（如超时、崩溃、无响应），并触发恰当的告警和恢复机制。

**为什么重要**：
- SubAgent 是分布式的，主 Agent 无法直接观察其内部状态
- 如果观察者失效，用户不知道 SubAgent 已崩溃，工作流卡住
- 误报告警会导致频繁的无谓中断

### 🔧 前置条件

1. 启动 Hivemind 和至少 1 个 Golem 工作节点

2. SubAgent 观察者配置：
   ```
   subagent.heartbeat_interval = 5s
   subagent.heartbeat_timeout = 15s
   subagent.max_retries = 3
   ```

3. 模拟故障工具已加载（见 `scripts/integration-tests/fixtures/fault-tools.json`）

### 📋 测试步骤

#### 步骤 1：创建主 Agent 和 SubAgent
```bash
# 创建主 Agent
curl -X POST http://localhost:11789/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "parent-agent",
    "model": "gpt-4",
    "system_prompt": "You are a coordinator."
  }'

# 创建子 Agent
curl -X POST http://localhost:11789/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "child-agent-1",
    "model": "gpt-4",
    "system_prompt": "You are a worker."
  }'
```

#### 步骤 2：主 Agent 发起 SubAgent 调用（正常情况）
```bash
curl -X POST http://localhost:11789/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "parent-agent",
    "messages": [
      {
        "role": "user",
        "content": "启动子任务处理数据"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "spawn_subagent",
          "description": "启动一个子智能体",
          "parameters": {
            "agent_id": "child-agent-1",
            "task": "process data"
          }
        }
      }
    ]
  }'
```

**预期**：SubAgent 按时响应，观察者记录正常心跳

#### 步骤 3：模拟 SubAgent 无响应（故障注入）
```bash
# 通过测试工具停止 SubAgent 的心跳发送
curl -X POST http://localhost:11789/v1/diagnostic/fault-inject \
  -H "Content-Type: application/json" \
  -d '{
    "fault_type": "stop_heartbeat",
    "target_agent": "child-agent-1",
    "duration_seconds": 20
  }'

# 主 Agent 再次尝试调用 SubAgent
curl -X POST http://localhost:11789/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "parent-agent",
    "messages": [{"role": "user", "content": "再次启动子任务"}]
  }'
```

**预期**：
- 第 1-3 次调用 → 心跳超时，重试
- 第 3 次重试失败 → 触发告警 `SUBAGENT_UNRESPONSIVE`
- 主 Agent 返回错误，建议降级处理

#### 步骤 4：验证告警和恢复
查看告警日志和恢复行为：
```bash
# 查看告警记录
grep "SUBAGENT_UNRESPONSIVE" ~/.echoryn/logs/subagent-observer.log

# 验证自动恢复（心跳恢复后）
curl -X POST http://localhost:11789/v1/diagnostic/fault-inject \
  -H "Content-Type: application/json" \
  -d '{
    "fault_type": "resume_heartbeat",
    "target_agent": "child-agent-1"
  }'

# 主 Agent 应该能再次成功调用 SubAgent
curl -X POST http://localhost:11789/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "parent-agent",
    "messages": [{"role": "user", "content": "子任务应该已恢复，再试一次"}]
  }'
```

### 📊 验证指标

| 指标 | 预期值 | 实际值 | 状态 | 备注 |
|------|-------|-------|------|------|
| **异常检测时间** | < 20s | ⏳ 待测 | | 从 SubAgent 故障到主 Agent 收到告警的延迟 |
| **告警误报率** | < 2% | ⏳ 待测 | | 正常运行中被误认为故障的比例 |
| **告警漏报率** | < 1% | ⏳ 待测 | | 实际故障未被检测的比例 |
| **自动恢复成功率** | > 95% | ⏳ 待测 | | 故障恢复后重新成功调用的比例 |
| **观察者开销** | < 3% | ⏳ 待测 | 额外 CPU% | 心跳监控对系统的占比 |

### ✅ 通过条件

满足以下**全部**条件时，场景通过：

- ✅ 异常检测时间 < 20s
- ✅ 告警误报率 < 2%
- ✅ 告警漏报率 < 1%
- ✅ 自动恢复成功率 > 95%
- ✅ 观察者开销 < 3%

### ❌ 失败排查路径

| 症状 | 排查步骤 | 可能原因 |
|------|---------|---------|
| **检测时间 > 20s** | 1. 检查 `heartbeat_timeout` 配置<br>2. 查看网络延迟<br>3. 检查观察者轮询频率 | 心跳超时阈值过大或网络问题 |
| **误报率 > 2%** | 1. 增加 `heartbeat_timeout`<br>2. 增加重试次数<br>3. 检查网络不稳定性 | 阈值过敏感 |
| **恢复成功率 < 95%** | 1. 检查重试逻辑<br>2. 查看日志中的失败原因<br>3. 验证 SubAgent 是否真的恢复 | 恢复逻辑有缺陷或 SubAgent 状态未正确更新 |

### 🔗 相关代码

- **观察者实现**：`internal/hivemind/service/subagent/observer.go`
- **故障注入 API**：`internal/hivemind/handler/diagnostic.go`
- **配置定义**：`conf/hivemind-server-test.json`

---

## 场景 3：上下文压缩精度

### 📌 测试目的

验证上下文压缩机制在 token 超限时，能否在保持对话连贯性和任务完成率的前提下，有效减少 token 消耗。

**为什么重要**：
- 长对话场景下，token 快速耗尽会导致 Agent 被迫中断
- 压缩精度差会导致 Agent "遗忘"重要信息，任务失败
- 压缩算法不优化会大幅增加延迟

### 🔧 前置条件

1. 启动 Hivemind，配置：
   ```
   context.compression.enabled = true
   context.compression.trigger_threshold = 0.7  # 当 token 占用 > 70% 时触发
   context.compression.target_reduction = 0.4   # 目标压缩掉 40% 的 token
   context.compression.max_iterations = 3       # 最多压缩 3 次
   ```

2. 对话数据集已准备（见 `scripts/integration-tests/data/long-conversation.jsonl`）

### 📋 测试步骤

#### 步骤 1：创建长对话 Agent
```bash
curl -X POST http://localhost:11789/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "longchat-agent",
    "model": "gpt-4",
    "system_prompt": "你是一个知识助手，能够记住对话历史并基于之前的信息进行推理。",
    "config": {
      "max_tokens": 4000,
      "temperature": 0.7
    }
  }'
```

#### 步骤 2：构造长对话，触发压缩
使用数据集中的对话轮次逐个发送：
```bash
python3 scripts/integration-tests/test-compression.py \
  --agent-id longchat-agent \
  --dataset scripts/integration-tests/data/long-conversation.jsonl \
  --max-turns 100 \
  --output-file .test-data/compression-result.json
```

测试脚本会：
1. 逐个发送 100 轮对话消息
2. 记录每一轮的 token 使用量
3. 记录何时触发了压缩
4. 验证压缩后对话是否仍可理解

#### 步骤 3：评估压缩效果
```bash
python3 scripts/integration-tests/analyze-compression.py \
  --result .test-data/compression-result.json \
  --baseline scripts/integration-tests/data/compression-baseline.json
```

这个脚本会计算：
- 压缩率（压缩后 token / 原始 token）
- 精度损失（Agent 遗忘重要信息的比例）
- 延迟增加（压缩耗时）
- 对比基线改进情况

### 📊 验证指标

| 指标 | 预期值 | 实际值 | 状态 | 备注 |
|------|-------|-------|------|------|
| **压缩率** | 0.55 ~ 0.65 | ⏳ 待测 | | 压缩后 token 占原始的比例，目标 40% 减少 |
| **精度保留率** | > 90% | ⏳ 待测 | | Agent 能回忆关键信息的比例 |
| **任务完成率** | > 95% | ⏳ 待测 | | 长对话中多步任务成功完成的比例 |
| **平均压缩延迟** | < 1s | ⏳ 待测 | | 每次压缩操作耗时 |
| **P95 压缩延迟** | < 3s | ⏳ 待测 | | 压缩延迟的 95 分位数 |

### ✅ 通过条件

满足以下**全部**条件时，场景通过：

- ✅ 压缩率 0.55 ~ 0.65（±5%）
- ✅ 精度保留率 > 90%
- ✅ 任务完成率 > 95%
- ✅ 平均压缩延迟 < 1s
- ✅ P95 压缩延迟 < 3s

### ❌ 失败排查路径

| 症状 | 排查步骤 | 可能原因 |
|------|---------|---------|
| **压缩率过高（> 0.7）** | 1. 检查 `target_reduction` 配置<br>2. 增加压缩迭代次数 | 压缩目标设置不激进 |
| **压缩率过低（< 0.5）** | 1. 检查是否过度压缩<br>2. 验证 LLM 摘要效果 | 压缩目标过激进，损伤精度 |
| **精度保留率 < 90%** | 1. 查看被压缩掉的内容<br>2. 手工检查摘要质量<br>3. 尝试不同的摘要 LLM | LLM 摘要能力不足 |
| **任务完成率 < 95%** | 1. 分析未完成的任务<br>2. 检查是否信息遗漏<br>3. 考虑降低压缩目标 | 压缩丢失了任务关键信息 |
| **压缩延迟 > 1s** | 1. 检查 LLM 响应时间<br>2. 分析是否有网络瓶颈<br>3. 考虑使用更快的模型 | LLM 调用性能不足 |

### 🔗 相关代码

- **压缩器实现**：`internal/hivemind/service/agents/domain/service/runtime/compaction/compressor.go`
- **测试脚本**：`scripts/integration-tests/test-compression.py`
- **分析脚本**：`scripts/integration-tests/analyze-compression.py`
- **配置定义**：`conf/hivemind-server-test.json`

---

## 运行所有集成测试

### 一键执行所有场景

```bash
make test.integration
```

这会依次运行：
1. 场景 1：工具循环检测 (~5 分钟)
2. 场景 2：SubAgent 异常检测 (~8 分钟)
3. 场景 3：上下文压缩精度 (~10 分钟)

**总耗时**：约 25 分钟

### 生成测试报告

```bash
make test.integration.report
```

报告包含：
- ✅/❌ 各场景通过/失败状态
- 📊 指标对比（实际值 vs 预期值）
- 📈 趋势对比（本次 vs 上次基线）
- ⚠️ 异常指标告警

输出文件：`.test-data/integration-test-report.md`

---

## 基线数据管理

### 初始化基线

第一次运行时建立基线：

```bash
make test.integration.baseline
```

这会在 `.test-data/baseline/` 下保存三个文件：
- `toolloop.baseline.json`
- `subagent.baseline.json`
- `compression.baseline.json`

### 对比当前运行

```bash
make test.integration.compare
```

输出对比结果，如：
```
工具循环检测 (场景 1)
  误报率: 1.2% (基线: 1.1%) ⚠️ 差 0.1%
  检测延迟: 420ms (基线: 380ms) ⚠️ 差 40ms
  
SubAgent 异常检测 (场景 2)
  检测时间: 18s (基线: 16s) ⚠️ 差 2s
  恢复成功率: 96% (基线: 97%) ⚠️ 差 1%
  
上下文压缩 (场景 3)
  压缩率: 0.62 (基线: 0.60) ✅ 改进 0.02
  精度保留率: 92% (基线: 91%) ✅ 改进 1%
```

### 更新基线

如果改进确实是预期的，可以更新基线：

```bash
make test.integration.baseline.update
```

---

## CI/CD 集成

### GitHub Actions 配置

在 `.github/workflows/integration-tests.yml`：

```yaml
name: Integration Tests

on:
  push:
    branches: [master, develop]
  pull_request:
    branches: [master]

jobs:
  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: 1.21
      
      - name: Run Integration Tests
        run: make test.integration
      
      - name: Generate Report
        if: always()
        run: make test.integration.report
      
      - name: Upload Report
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: integration-test-report
          path: .test-data/integration-test-report.md
      
      - name: Comment on PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@v6
        with:
          script: |
            const fs = require('fs');
            const report = fs.readFileSync('.test-data/integration-test-report.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: report
            });
```

---

## 常见问题

### Q: 运行集成测试需要多长时间？
**A**: 约 25 分钟（三个场景并行可优化到 15 分钟）。建议在 CI 中与单元测试分离。

### Q: 如何本地快速验证？
**A**: 运行单个场景而不是全部：
```bash
make test.integration.toolloop      # 只测场景 1
make test.integration.subagent      # 只测场景 2
make test.integration.compression   # 只测场景 3
```

### Q: 指标基线如何维护？
**A**:
- 新功能 → 先运行一次建立基线
- 性能优化后 → 对比基线，改进则 `update` 基线
- 回归问题 → 对比基线找出哪个指标劣化

### Q: 测试失败了怎么办？
**A**: 参考各场景的"失败排查路径"表格，按步骤诊断。必要时可运行：
```bash
echoctl diag --verbose  # 查看详细诊断信息
```

---

## 后续扩展

当实现新的 Harness Engineering 机制时，在此文档增加新场景：

- [ ] **场景 4**：多渠道路由正确性（IM 网关故障转移）
- [ ] **场景 5**：插件冲突解决（插槽机制验证）
- [ ] **场景 6**：记忆系统一致性（FTS5 + 向量搜索混合检索）
- [ ] **场景 7**：团队协作消息顺序性（MessageBus 可靠投递）

每个新场景都应包含：测试目的 → 前置条件 → 测试步骤 → 验证指标 → 失败排查路径。
