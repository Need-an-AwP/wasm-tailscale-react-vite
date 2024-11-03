import { sessionStateStorage } from "./lib/js-state-store"


const usingPrivateController = false;

export async function initIPN() {
    if (typeof window.Go === 'undefined') {
        console.log('Waiting for Go...');
        return null;
    }

    try {
        const go = new window.Go();
        const result = await WebAssembly.instantiateStreaming(
            fetch("main.wasm"),
            go.importObject
        );

        go.run(result.instance).then(() => {
            console.error("Unexpected shutdown");
        });
        let ipn;
        if (usingPrivateController) {
            ipn = window.newIPN({
                stateStorage: sessionStateStorage,
                authKey: import.meta.env.VITE_PRIVATE_CONTROLLER_AUTH_KEY,
                hostname: 'wasm-tsconnect-testNode',
                controlURL: import.meta.env.VITE_CONTROL_URL
            });
        } else {
            ipn = window.newIPN({
                stateStorage: sessionStateStorage,
                authKey: import.meta.env.VITE_NODE_AUTH_KEY,
                hostname: 'wasm-tsconnect-testNode',
            });
        }

        return ipn;
    } catch (err) {
        console.error('WASM 加载错误:', err);
        return null;
    }
}