import React, { useMemo, useState } from 'react';

const DomainFilter = ({ domains, selectedDomains, onDomainToggle }) => {
    const [isExpanded, setIsExpanded] = useState(true);

    // Sort domains by count (descending)
    const sortedDomains = useMemo(() => {
        return [...domains].sort((a, b) => b.count - a.count);
    }, [domains]);

    const handleSelectAll = () => {
        sortedDomains.forEach(d => {
            if (!selectedDomains.has(d.name)) {
                onDomainToggle(d.name);
            }
        });
    };

    const handleClearAll = () => {
        selectedDomains.forEach(domainName => {
            onDomainToggle(domainName);
        });
    };

    if (domains.length === 0) return null;

    return (
        <div className="domain-filter">
            <div className="domain-filter-header" onClick={() => setIsExpanded(!isExpanded)}>
                <h3>Filter by Domain</h3>
                <span className="toggle-icon">{isExpanded ? '▼' : '▶'}</span>
            </div>

            {isExpanded && (
                <div className="domain-filter-content">
                    <div className="domain-filter-actions">
                        <button className="btn-link" onClick={handleSelectAll}>
                            Select All
                        </button>
                        <span>•</span>
                        <button className="btn-link" onClick={handleClearAll}>
                            Clear All
                        </button>
                        {selectedDomains.size > 0 && (
                            <span className="filter-count">
                                ({selectedDomains.size} selected)
                            </span>
                        )}
                    </div>

                    <div className="domain-list">
                        {sortedDomains.map(domain => (
                            <label key={domain.name} className="domain-item">
                                <input
                                    type="checkbox"
                                    checked={selectedDomains.has(domain.name)}
                                    onChange={() => onDomainToggle(domain.name)}
                                />
                                <span className="domain-name">{domain.name}</span>
                                <span className="domain-count">({domain.count})</span>
                            </label>
                        ))}
                    </div>
                </div>
            )}
        </div>
    );
};

export default DomainFilter;
