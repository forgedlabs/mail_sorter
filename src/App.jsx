import React, { useState, useCallback, useRef, useMemo } from 'react';
import InputArea from './components/InputArea';
import ResultsTable from './components/ResultsTable';
import { processBatch, validateWithServer } from './utils/validator';

function App() {
  const [inputText, setInputText] = useState('');
  const [results, setResults] = useState([]);
  const [processingState, setProcessingState] = useState('IDLE'); // IDLE, PROCESSING, PAUSED
  const [progress, setProgress] = useState({ current: 0, total: 0 });
  const [useServer, setUseServer] = useState(false);
  const [selectedDomains, setSelectedDomains] = useState(new Set());
  const stopProcessingRef = useRef(false);

  // Extract unique domains from results
  const domains = useMemo(() => {
    const domainMap = new Map();
    results.forEach(result => {
      if (result.domain) {
        const count = domainMap.get(result.domain) || 0;
        domainMap.set(result.domain, count + 1);
      }
    });
    return Array.from(domainMap.entries()).map(([name, count]) => ({ name, count }));
  }, [results]);

  // Toggle domain selection
  const handleDomainToggle = (domainName) => {
    setSelectedDomains(prev => {
      const newSet = new Set(prev);
      if (newSet.has(domainName)) {
        newSet.delete(domainName);
      } else {
        newSet.add(domainName);
      }
      return newSet;
    });
  };

  // Get filtered results based on selected domains
  const getFilteredResults = () => {
    if (selectedDomains.size === 0) {
      return results;
    }
    return results.filter(r => selectedDomains.has(r.domain));
  };

  const handleStop = () => {
    stopProcessingRef.current = true;
    setProcessingState('PAUSED');
  };

  const processQueue = async (itemsToProcess) => {
    stopProcessingRef.current = false;
    setProcessingState('PROCESSING');

    let completedCount = progress.current;

    // Helper to update a single result
    const updateResult = (email, serverResult) => {
      setResults(prevResults => prevResults.map(r => {
        if (r.email === email) {
          return { ...r, ...serverResult };
        }
        return r;
      }));
    };

    // Process sequentially (or with limited concurrency) to allow easy stopping
    for (const item of itemsToProcess) {
      if (stopProcessingRef.current) {
        break;
      }

      try {
        const serverResponse = await validateWithServer([item.email]);
        if (serverResponse && serverResponse.length > 0) {
          updateResult(item.email, serverResponse[0]);
        } else {
          updateResult(item.email, { isValid: false, reason: 'Server Error' });
        }
      } catch (e) {
        console.error(e);
        updateResult(item.email, { isValid: false, reason: 'Network Error' });
      }

      completedCount++;
      setProgress(prev => ({ ...prev, current: completedCount }));
    }

    if (!stopProcessingRef.current) {
      setProcessingState('IDLE');
    }
  };

  const handleProcess = async () => {
    if (processingState === 'PROCESSING') {
      handleStop();
      return;
    }

    if (processingState === 'PAUSED') {
      // Resume: Find pending items
      const pendingItems = results.filter(r => r.reason === 'Pending...');
      if (pendingItems.length > 0) {
        await processQueue(pendingItems);
      } else {
        setProcessingState('IDLE');
      }
      return;
    }

    // Start Fresh
    const clientResults = processBatch(inputText);

    if (!useServer) {
      setResults(clientResults);
      return;
    }

    // Initialize Results
    const initialResults = clientResults.map(r =>
      r.isValid ? { ...r, isValid: true, reason: 'Pending...' } : r
    );
    setResults(initialResults);

    const validItems = clientResults.filter(r => r.isValid);
    if (validItems.length === 0) {
      return;
    }

    setProgress({ current: 0, total: validItems.length });
    await processQueue(validItems);
  };

  const handleExportCSV = () => {
    const filteredResults = getFilteredResults();
    if (filteredResults.length === 0) return;
    const headers = 'Email,Status,Domain,Reason\n';
    const rows = filteredResults.map(r => `${r.email},${r.isValid ? 'VALID' : 'INVALID'},${r.domain},${r.reason}`).join('\n');
    const blob = new Blob([headers + rows], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'email_results.csv';
    a.click();
  };

  const handleExportValid = () => {
    const filteredResults = getFilteredResults();
    if (filteredResults.length === 0) return;
    const validEmails = filteredResults.filter(r => r.isValid).map(r => r.email).join('\n');
    const blob = new Blob([validEmails], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'valid_emails.txt';
    a.click();
  };

  return (
    <div className="layout-split">
      <InputArea
        value={inputText}
        onChange={setInputText}
        onProcess={handleProcess}
        useServer={useServer}
        setUseServer={setUseServer}
        processingState={processingState}
        progress={progress}
      />
      <ResultsTable
        results={results}
        onExportCSV={handleExportCSV}
        onExportValid={handleExportValid}
        domains={domains}
        selectedDomains={selectedDomains}
        onDomainToggle={handleDomainToggle}
      />
    </div>
  );
}

export default App;
