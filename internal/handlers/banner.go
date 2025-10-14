package handlers

// ConnectionBannerHTML returns the HTML and CSS for the HA connection status banner
func ConnectionBannerHTML() string {
	return `
    <style>
        .ha-connection-banner {
            display: none;
            background: #fee2e2;
            border-bottom: 3px solid #ef4444;
            padding: 12px 20px;
            text-align: center;
            font-weight: bold;
            color: #991b1b;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            z-index: 9999;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .ha-connection-banner.show {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 15px;
        }
        .ha-connection-banner-icon {
            font-size: 20px;
        }
        .ha-connection-banner-retry {
            background: #991b1b;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
            font-weight: bold;
        }
        .ha-connection-banner-retry:hover {
            background: #7f1d1d;
        }
        .ha-connection-banner-retry:disabled {
            background: #9ca3af;
            cursor: not-allowed;
        }
    </style>
    <div id="haConnectionBanner" class="ha-connection-banner">
        <span class="ha-connection-banner-icon">⚠️</span>
        <span>Home Assistant connection lost. NFC pairing and playback unavailable.</span>
        <button id="haRetryBtn" class="ha-connection-banner-retry">Retry Connection</button>
    </div>
`
}

// ConnectionBannerScript returns the JavaScript for polling HA connection status
func ConnectionBannerScript() string {
	return `
    <script>
        (function() {
            const banner = document.getElementById('haConnectionBanner');
            const retryBtn = document.getElementById('haRetryBtn');
            let wasDisconnected = false;

            function checkHAConnection() {
                fetch('/api/status/ha')
                    .then(response => response.json())
                    .then(data => {
                        if (!data.connected) {
                            banner.classList.add('show');
                            wasDisconnected = true;
                        } else {
                            banner.classList.remove('show');
                            // If we were disconnected and now reconnected, reload the page
                            if (wasDisconnected) {
                                console.log('HA reconnected, reloading page...');
                                window.location.reload();
                            }
                        }
                    })
                    .catch(error => {
                        console.error('Failed to check HA status:', error);
                        banner.classList.add('show');
                        wasDisconnected = true;
                    });
            }

            // Retry connection button handler
            retryBtn.addEventListener('click', function() {
                retryBtn.disabled = true;
                retryBtn.textContent = 'Retrying...';

                fetch('/api/status/ha/reconnect', { method: 'POST' })
                    .then(response => response.json())
                    .then(data => {
                        if (data.success) {
                            console.log('Reconnection successful');
                            // Check status immediately
                            checkHAConnection();
                        } else {
                            console.error('Reconnection failed:', data.message);
                            alert('Failed to reconnect: ' + data.message);
                        }
                        retryBtn.disabled = false;
                        retryBtn.textContent = 'Retry Connection';
                    })
                    .catch(error => {
                        console.error('Retry request failed:', error);
                        alert('Failed to send retry request');
                        retryBtn.disabled = false;
                        retryBtn.textContent = 'Retry Connection';
                    });
            });

            // Check immediately on load
            checkHAConnection();

            // Poll every 5 seconds
            setInterval(checkHAConnection, 5000);
        })();
    </script>
`
}
