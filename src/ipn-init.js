import { sessionStateStorage } from "./lib/js-state-store"


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

        const ipn = window.newIPN({
            stateStorage: sessionStateStorage,
            authKey: 'tskey-auth-kosWk4uE3311CNTRL-1PrBU5cG4UHkaesJoZyCUHHtpRnLs2uD',
            hostname: 'wasm-tsconnect-testNode',
        });

        return ipn;
    } catch (err) {
        console.error('WASM 加载错误:', err);
        return null;
    }
}