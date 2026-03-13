# logutil

统一日志初始化。

```go
import "github.com/cocomhub/download-manager/pkg/logutil"

logutil.InitLogger(logutil.LogConfig{
  Level:   "info",
  Console: true,
})
```
