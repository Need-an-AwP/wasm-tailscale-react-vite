# tailscale web assembly rewrite

这是一个tailscale客户端的wasm实现
核心go代码重写来自https://github.com/tailscale/tailscale/tree/main/cmd/tsconnect 的wasm/wasm_js.go

- 没有wasm-opt优化
- 没有构建版本信息

在项目根目录运行`npm run build:wasm`来构建wasm文件并复制到public目录

相较原始的tsconnect，这个重写的版本中添加了单独的netCheck方法，用于获取derp延迟信息

作用类似tailscale cli中的
```bash 
tailscale netcheck
```