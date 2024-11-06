import { sessionStateStorage } from "./lib/js-state-store"



export async function initIPN({ wsCallback }) {
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
        const ipn = window.newIPN({
            stateStorage: sessionStateStorage,
            authKey: import.meta.env.VITE_NODE_AUTH_KEY,
            hostname: 'wasm-tsconnect-testNode',
            wsCallback: wsCallback
        });


        return ipn;
    } catch (err) {
        console.error('WASM 加载错误:', err);
        return null;
    }
}