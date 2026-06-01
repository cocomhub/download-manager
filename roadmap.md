download-manager — 多源下载管理器

 当前规模：~14K 行 Go，14 个外部依赖，4 种任务类型，3 种存储后端，2 种下载器

 该项目的 self-documented
 roadmap（docs/implementation-roadmap.md）框架质量很高，此处在此基础上补充可操作细节。

 P0 基础治理（优先级最高）

 1. 统一写操作保护（roadmap P0-1，影响安全性）
   - 当前：wrapWrite 中间件覆盖不全，POST /api/tasks、POST /api/config/* 等绕过
   - 动作：
       - 抽象 writeGuard 中间件，全局注册到所有写路由
     - 为 api/server_write_guard_test.go 增加覆盖率（当前仅基础 checks）
     - 验证 UI 模式下所有写接口返回一致语义（如 405 Method Not Allowed）
 2. 优雅停机完善（roadmap P0-3，影响状态一致性）
   - 实现 http.Server.Shutdown() 带超时 context
   - 等待 worker goroutine 退出（workerWg.Wait()）
   - 调用 FileStorage.ForceFlush() 落盘缓存
   - 标记所有 in-flight downloading 对象为 failed/pending 以便重启恢复
 3. 下载路径一致性（roadmap P0-2）
   - 收敛 GetDownloadRootDir() 到单一配置来源
   - /files/ 路径与下载落盘目录使用同一计算逻辑

 P1 性能与维护性

 4. 聚合查询优化（roadmap P1-4）
   - AggregateObjects 全量拉取→内存过滤排序分页 → 推送到存储层
   - 存储后端（file/mongo）实现 Search() 时支持 limit/offset/sort/filter 下推
   - 5000+ 对象场景的基准测试
 5. 配置接口补齐（roadmap P1-5）
   - GET /api/config 当前返回有限字段
   - 补齐 filesystem、http、proxy、progress、ffmpeg 的读取/编辑
 6. 前端模块化（roadmap P1-6）
   - 当前单文件 ~4050 行 index.html（Vue 3 + Tailwind CDN）
   - 建议策略（渐进式）：
     - 第一阶段：拆主文件为 3-4 个独立 JS 模块（taskList.js、aggregateView.js、configPanel.js）     
     - 第二阶段：引入 Vite 构建流程，实现 SFC 组件

 P2 架构演进

 7. Manager 职责拆分（roadmap P2-8，~1500 行需拆分）
   - 抽离：AggregationService（聚合）、ObjectController（对象控制）、ConfigService（配置治理）        
   - Manager 保留：扫描、调度、worker 池、生命周期协调
   - 建议顺序：先抽 ConfigService（接口边界最清晰）→ ObjectController → AggregationService
 8. 任务模板与站点 Adapter（roadmap P2-9）
   - 当前 vikacg/tktube/hanime 有大量重复的 paging/refreshing/caching 逻辑
   - 沉淀 PagingScanner、DetailFetcher、CacheManager 通用模板
   - 新增任务时只需编写 site-specific adapter（类似 collector 模式）
 9. 对象扩展字段协议（roadmap P2-10）
   - Extra map[string]any、Metadata map[string]string 中散落
 content_group、preview_url、local_preview、tags、files 等
   - 建立 model/ObjectMeta 结构化字段，逐步取代隐式 map 协议

 P3 工程基础设施

 10. 建立 CI（.github/workflows/ci.yml）
   - go test ./... 自动运行
   - 可选：golangci-lint（配合 .golangci.yml）
 11. DomainLimiter 忙等待修复
   - pkg/dlcore/client.go 中的 for max != 0 && d.cur[host] >= max { mu.Unlock/Lock } 是 spin-loop     
   - 改为 sync.Cond 或 channel-based semaphore