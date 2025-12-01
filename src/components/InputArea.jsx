import React from 'react';

const InputArea = ({ value, onChange, onProcess, useServer, setUseServer, processingState, progress }) => {

    const getButtonText = () => {
        if (processingState === 'PROCESSING') {
            return `Stop ${progress?.total ? `(${progress.current}/${progress.total})` : ''} `;
        }
        if (processingState === 'PAUSED') {
            return `Resume(${progress?.current} / ${progress?.total})`;
        }
        return 'Validate & Sort';
    };

    const getButtonClass = () => {
        if (processingState === 'PROCESSING') return 'btn btn-danger'; // Assuming we add a danger class or style
        if (processingState === 'PAUSED') return 'btn btn-success';
        return 'btn btn-primary';
    };

    return (
        <div className="panel panel-left">
            <div className="header-bar">
                <h2>Input Emails</h2>
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <label style={{ fontSize: '0.85rem', display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer', fontWeight: '600', textTransform: 'uppercase' }}>
                        <input
                            type="checkbox"
                            checked={useServer}
                            onChange={(e) => setUseServer(e.target.checked)}
                            disabled={processingState !== 'IDLE'}
                            style={{ accentColor: 'black', width: '16px', height: '16px' }}
                        />
                        Smart Verify
                    </label>
                    <button
                        className={getButtonClass()}
                        onClick={onProcess}
                    >
                        {getButtonText()}
                    </button>
                </div>
            </div>
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <textarea
                    placeholder="PASTE EMAILS HERE..."
                    value={value}
                    onChange={(e) => onChange(e.target.value)}
                    spellCheck="false"
                />
                <div style={{ marginTop: '12px', fontSize: '0.75rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                    Supports CSV, TXT, and raw text.
                </div>
            </div>
        </div>
    );
};

export default InputArea;
