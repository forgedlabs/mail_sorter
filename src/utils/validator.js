// Strict email regex
// Matches: local-part @ domain . tld
const EMAIL_REGEX = /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;

export function validateEmail(email) {
    if (!email) return { isValid: false, reason: 'Empty' };

    const trimmed = email.trim();
    if (trimmed.length === 0) return { isValid: false, reason: 'Empty' };

    if (!trimmed.includes('@')) return { isValid: false, reason: 'Missing @' };

    if (trimmed.length > 254) return { isValid: false, reason: 'Too long' };

    const isValid = EMAIL_REGEX.test(trimmed);

    let domain = '';
    if (isValid) {
        domain = trimmed.split('@')[1].toLowerCase();
    } else {
        // Try to extract domain even if invalid, for sorting purposes if possible
        const parts = trimmed.split('@');
        if (parts.length === 2) domain = parts[1].toLowerCase();
    }

    return {
        email: trimmed,
        isValid,
        domain,
        reason: isValid ? 'Valid' : 'Invalid Format'
    };
}

export function processBatch(text) {
    const rawLines = text.split(/[\n,;]+/).map(s => s.trim()).filter(s => s.length > 0);
    const uniqueSet = new Set();
    const results = [];

    for (const line of rawLines) {
        if (uniqueSet.has(line)) continue; // Deduplicate
        uniqueSet.add(line);
        results.push(validateEmail(line));
    }

    return results;
}

export async function validateWithServer(emails) {
    try {
        // Use production Go backend or environment variable
        const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';
        const response = await fetch(`${API_URL}/v1/validate/batch`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ emails })
        });

        if (!response.ok) throw new Error('Server error');

        const data = await response.json();

        // Map Go backend response to frontend format
        return data.results.map(result => ({
            email: result.email,
            domain: result.domain,
            isValid: result.status === 'valid',
            status: result.status, // Pass raw status for granular display
            reason: formatReason(result),
            isCatchAll: result.is_catch_all,
            confidence: result.confidence,
            smtpCode: result.smtp_code
        }));
    } catch (error) {
        console.error('Validation error:', error);
        // Fallback to client-side validation results if server fails
        return emails.map(email => ({ ...validateEmail(email), reason: 'Server Error' }));
    }
}

// Helper to format reason from Go backend response
function formatReason(result) {
    if (result.status === 'valid') {
        return result.is_catch_all ? 'Catch-All Domain (Risky)' : 'Mailbox Exists';
    } else if (result.status === 'invalid') {
        return result.reason === 'mailbox_not_found' ? 'User Not Found' :
            result.reason === 'no_mx_records' ? 'No MX Records' :
                'Invalid';
    } else if (result.status === 'catch-all') {
        return 'Catch-All Domain (Risky)';
    } else if (result.status === 'unknown') {
        return 'SMTP Timeout/Error';
    } else if (result.status === 'risky') {
        return 'Disposable/Risky Domain';
    }
    return result.reason || 'Unknown';
}

