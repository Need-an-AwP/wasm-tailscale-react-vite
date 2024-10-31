import { useEffect, useState, useRef } from 'react'
import './App.css'
import IPNStatus from './components/IPNStatus'
import { initIPN } from './ipn-init'

function App() {
    const [ipn, setIpn] = useState(null);
    const initRef = useRef(false);

    useEffect(() => {
        const initIPNInstance = async () => {
            if (initRef.current) return;
            initRef.current = true;
            
            const ipnInstance = await initIPN();
            if (ipnInstance) {
                setIpn(ipnInstance);
            }
        }

        initIPNInstance();
    }, []);

    return (
        <>
            <IPNStatus ipn={ipn} />
        </>
    )
}

export default App
