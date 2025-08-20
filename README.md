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

在运行`npm run dev`前，你需要在项目根目录创建`.env`文件，其中需要包含以下变量

```
VITE_NODE_AUTH_KEY=*your authkey*
```
~~启用或禁用连接到私有controller，修改[src/ipn-init.js](./src/ipn-init.js#L4)中的`usingPrivateController`变量~~
不再尝试使用headscale生成的authkey，仅使用tailscale官方控制器生成的authkey

使用[src/components/IPNStatus.jsx](./src/components/IPNStatus.jsx)中的IPNStatus组件来查看当前连接状态，以及使用fetch方法和发起ws连接