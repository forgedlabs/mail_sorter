import React, { useMemo } from 'react';
import DomainFilter from './DomainFilter';
// import * as ReactWindow from 'react-window';
// const List = ReactWindow.FixedSizeList;
// import AutoSizer from 'react-virtualized-auto-sizer';

const Row = ({ index, style, data }) => {
    const item = data[index];
    const statusColor = item.reason === 'Pending...'
        ? 'var(--text-secondary)'
        : (item.status === 'unknown'
            ? 'var(--unknown-color)'
            : (item.isValid
                ? (item.isCatchAll ? 'var(--warning-color)' : 'var(--success-color)')
                : 'var(--error-color)'));

    const statusText = item.reason === 'Pending...'
        ? 'PENDING'
        : (item.status === 'unknown'
            ? 'UNKNOWN'
            : (item.isValid
                ? (item.isCatchAll ? 'CATCH-ALL' : 'VALID')
                : 'INVALID'));

    return (
        <div style={{ ...style, display: 'flex', alignItems: 'center', borderBottom: '1px solid #eee', padding: '12px 16px', fontSize: '0.85rem', fontFamily: 'var(--font-mono)' }}>
            <div style={{ width: '40%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={item.email}>
                {item.email}
            </div>
            <div
                style={{ width: '20%', color: statusColor, fontWeight: '700', textTransform: 'uppercase', cursor: item.status === 'unknown' ? 'help' : 'default' }}
                title={item.status === 'unknown' ? 'SMTP Timeout - Likely blocked by provider' : ''}
            >
                {statusText}
            </div>
            <div style={{ width: '20%', color: 'var(--text-secondary)' }}>
                {item.domain || '-'}
            </div>
            <div style={{ flex: 1, color: 'var(--text-secondary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {item.reason}
            </div>
        </div>
    );
};

const ResultsTable = ({ results, onExportCSV, onExportValid, domains, selectedDomains, onDomainToggle }) => {
    // Filter results based on selected domains
    const filteredResults = useMemo(() => {
        if (selectedDomains.size === 0) {
            return results;
        }
        return results.filter(r => selectedDomains.has(r.domain));
    }, [results, selectedDomains]);

    return (
        <div className="panel panel-right" style={{ padding: 0 }}>
            <div className="header-bar" style={{ padding: '16px', borderBottom: '2px solid black', margin: 0 }}>
                <h2>
                    Results {results.length > 0 && `(${filteredResults.length}${selectedDomains.size > 0 ? ` / ${results.length}` : ''})`}
                </h2>
                <div style={{ display: 'flex', gap: '8px' }}>
                    <button className="btn" onClick={onExportCSV} disabled={filteredResults.length === 0}>Export CSV</button>
                    <button className="btn" onClick={onExportValid} disabled={filteredResults.length === 0}>Export Valid</button>
                </div>
            </div>

            {domains.length > 0 && (
                <DomainFilter
                    domains={domains}
                    selectedDomains={selectedDomains}
                    onDomainToggle={onDomainToggle}
                />
            )}

            <div style={{ flex: 1, overflow: 'auto' }}>
                {filteredResults.map((item, index) => (
                    <Row key={index} index={index} data={filteredResults} style={{}} />
                ))}
            </div>
        </div>
    );
};

export default ResultsTable;
