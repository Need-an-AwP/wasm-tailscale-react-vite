# tailscale web assembly rewrite

This is a wasm implementation of the tailscale client.
The core Go code is rewritten from https://github.com/tailscale/tailscale/tree/main/cmd/tsconnect's wasm/wasm_js.go

- No wasm-opt optimization
- No build version information

Run `npm run build:wasm` in the project root directory to build the wasm file and copy it to the public directory

Compared to the original tsconnect, this rewritten version adds a separate netCheck method to obtain derp latency information

Similar to tailscale cli's
```bash
tailscale netcheck
```

Before running `npm run dev`, you need to create a `.env` file in the project root directory, which needs to contain the following variables

```
VITE_NODE_AUTH_KEY=*tailscale authkey from offical controller*
VITE_PRIVATE_CONTROLLER_AUTH_KEY=*authkey from private controller*
VITE_CONTROL_URL=*private controller url*
```
To enable or disable connection to private controller, modify the `usingPrivateController` variable in [src/ipn-init.js](./src/ipn-init.js#L4)

---

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
VITE_NODE_AUTH_KEY=*tailscale authkey from offical controller*
VITE_PRIVATE_CONTROLLER_AUTH_KEY=*authkey from private controller*
VITE_CONTROL_URL=*private controller url*
```
启用或禁用连接到私有controller，修改[src/ipn-init.js](./src/ipn-init.js#L4)中的`usingPrivateController`变量
