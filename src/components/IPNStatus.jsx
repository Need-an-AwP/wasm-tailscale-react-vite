import React, { useState, useEffect, useRef } from 'react';
import { initIPN } from '../ipn-init';

const IPNStatus = () => {
    const [ipn, setIpn] = useState(null);
    const initRef = useRef(false);

    useEffect(() => {
        const wsCallback = (message) => {
            try {
                const data = JSON.parse(message);
                // 在这里处理接收到的消息
                console.log("Received JSON message via callback at ws server:", data);
            } catch (e) {
                console.log("Received raw message at ws server:", message);
            }
        };

        const initIPNInstance = async () => {
            if (initRef.current) return;
            initRef.current = true;

            const ipnInstance = await initIPN({ wsCallback });
            if (ipnInstance) {
                setIpn(ipnInstance);
            }
        }

        initIPNInstance();
    }, []);

    const [ipnState, setIpnState] = useState("NoState");
    const [netMap, setNetMap] = useState(null);
    const [browseToURL, setBrowseToURL] = useState(null);
    const [goPanicError, setGoPanicError] = useState(null);
    const [netCheckResult, setNetCheckResult] = useState(null);
    const [isChecking, setIsChecking] = useState(false);
    const [isHTTPServerRunning, setIsHTTPServerRunning] = useState(false);

    useEffect(() => {
        if (!ipn) return;

        ipn.run({
            notifyState: handleIPNState,
            notifyNetMap: handleNetMap,
            notifyBrowseToURL: handleBrowseToURL,
            notifyPanicRecover: handleGoPanic,
        });
    }, [ipn]);

    const handleIPNState = async (state) => {
        setIpnState(state);
        if (state === "NeedsLogin") {
            ipn?.login();
        } else if (["Running", "NeedsMachineAuth"].includes(state)) {
            setBrowseToURL(undefined);
            if (!isHTTPServerRunning) {
                const res = await ipn.startHTTPServer(8848)
                console.log(res)
                setIsHTTPServerRunning(true);
            }
        }
    };

    const handleNetMap = (netMapStr) => {
        try {
            const netMapData = JSON.parse(netMapStr);
            setNetMap(netMapData);
            console.log("Network Map:", netMapData);
        } catch (err) {
            console.error("Failed to parse netMap:", err);
        }
    };

    const handleBrowseToURL = (url) => {
        if (ipnState === "Running") return;
        setBrowseToURL(url);
    };

    const handleGoPanic = (error) => {
        console.error("Go panic:", error);
        setGoPanicError(error);
        setTimeout(() => setGoPanicError(null), 10000);
    };

    const handleNetCheck = async () => {
        setIsChecking(true);

        try {
            const result = await ipn.netCheck();
            const data = JSON.parse(result);
            console.log(data)
            setNetCheckResult(data);
        } catch (err) {
            console.log(err.toString());
        } finally {
            setIsChecking(false);
        }
    };

    return (
        <div className="ipn-status">
            {/* 状态显示 */}
            <div className="status-badge">
                ipnstate:{ipnState}
            </div>

            <button onClick={handleNetCheck}>
                netCheck
            </button>

            {/* 错误显示 */}
            {goPanicError && (
                <div className="error-message">
                    <p>Error: {goPanicError}</p>
                </div>
            )}

            {/* 授权 URL 显示 */}
            {browseToURL && (
                <div className="auth-url">
                    <p>Please authenticate at:</p>
                    <a href={browseToURL} target="_blank" rel="noopener noreferrer">
                        {browseToURL}
                    </a>
                </div>
            )}

            {/* 机器认证提示 */}
            {ipnState === "NeedsMachineAuth" && (
                <div className="auth-message">
                    <p>An administrator needs to approve this device.</p>
                </div>
            )}

            {/* 网络状态显示 */}
            {netMap && ipnState === "Running" && (
                <div className="network-status" style={{ display: 'flex', flexDirection: 'row', gap: '10px' }}>
                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                        <h3>peers</h3>
                        {netMap.peers.map((peer) => (
                            <p>
                                name: {peer.name}<br />
                                addresses: {peer.addresses.join(', ')}<br />
                                online: {peer.online ? 'true' : 'false'}<br />
                                <button onClick={async () => {
                                    ipn.fetch(`http://${peer.addresses[0]}:8848/hello`)
                                        .then(res => res.text())
                                        .then(text => console.log(text))
                                }}>
                                    fetch this peer on /hello
                                </button>
                                <button onClick={async () => {
                                    // 使用Tailscale网络中的WebSocket连接
                                    const ws = new WebSocket(`ws://${peer.addresses[0]}:8848/ws`);

                                    ws.onopen = () => {
                                        console.log(`WebSocket connected to \nws://${peer.addresses[0]}:8848/ws`);
                                        // 发送测试消息
                                        setInterval(() => {
                                            console.log(`send message to ${peer.addresses[0]}`)
                                            ws.send(JSON.stringify({
                                                type: 'hello',
                                                from: netMap.self.name,
                                                message: `Hello from ${netMap.self.addresses[0]} in ws !`
                                            }));
                                        }, 1000);
                                    };

                                    ws.onmessage = (event) => {
                                        try {
                                            const data = JSON.parse(event.data);
                                            console.log(`Received message from ${peer.name}:`, data);
                                        } catch (e) {
                                            console.log(`Received raw message from ${peer.name}:`, event.data);
                                        }
                                    };

                                    ws.onerror = (error) => {
                                        console.error(`WebSocket error with ${peer.name}:`, error);
                                    };

                                    ws.onclose = () => {
                                        console.log(`WebSocket connection with ${peer.name} closed`);
                                    };
                                }}>
                                    connect ws
                                </button>
                            </p>
                        ))}
                    </div>

                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                        <h3>self</h3>
                        <p>name: {netMap.self.name}</p>
                        <p>addresses: {netMap.self.addresses.join(', ')}</p>
                    </div>
                </div>
            )}

            {/* Tailnet Lock 提示 */}
            {netMap?.lockedOut && (
                <div className="locked-message">
                    <p>
                        This instance needs to be signed due to tailnet lock being enabled.
                        Run the following command on a trusted device:
                    </p>
                    <pre>tailscale lock sign {netMap.self?.nodeKey}</pre>
                </div>
            )}
        </div>
    );
};

export default IPNStatus;