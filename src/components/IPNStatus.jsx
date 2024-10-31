import React, { useState, useEffect } from 'react';

const IPNStatus = ({ ipn }) => {
    const [ipnState, setIpnState] = useState("NoState");
    const [netMap, setNetMap] = useState(null);
    const [browseToURL, setBrowseToURL] = useState(null);
    const [goPanicError, setGoPanicError] = useState(null);
    const [netCheckResult, setNetCheckResult] = useState(null);
    const [isChecking, setIsChecking] = useState(false);

    useEffect(() => {
        if (!ipn) return;

        ipn.run({
            notifyState: handleIPNState,
            notifyNetMap: handleNetMap,
            notifyBrowseToURL: handleBrowseToURL,
            notifyPanicRecover: handleGoPanic,
        });
    }, [ipn]);

    const handleIPNState = (state) => {
        setIpnState(state);
        if (state === "NeedsLogin") {
            ipn?.login();
        } else if (["Running", "NeedsMachineAuth"].includes(state)) {
            setBrowseToURL(undefined);
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
            <div className="status-header">
                <h2>Tailscale Status</h2>
                <div className="status-badge">
                    {ipnState}
                </div>
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
                            <>
                                <p>name: {peer.name}</p>
                                <p>addresses: {peer.addresses.join(', ')}</p>
                            </>
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