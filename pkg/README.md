# pkg 概览

该目录聚合项目内可复用的通用能力，提供稳定、简洁的 API。

- logutil：统一日志初始化与配置
- errors：错误包装与判定
- retry：带指数退避的重试
- config：环境变量与 JSON 加载
- httpx：带超时与 Context 的 HTTP 客户端
- fs：文件与路径辅助
- concurrency：最小工作池

示例：

```go
import (
	"context"
	"time"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/pkg/retry"
)

func main() {
	logutil.InitLogger(logutil.LogConfig{Level: "info", Console: true})
	retry.Do(context.Background(), func() error {
		return nil
	}, retry.Attempts(3), retry.InitialDelay(200*time.Millisecond))
}
```
