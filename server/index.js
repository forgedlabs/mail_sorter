const express = require('express');
const cors = require('cors');
const dns = require('dns').promises;
const net = require('net');

const app = express();
const PORT = 3001;

app.use(cors());
app.use(express.json({ limit: '10mb' }));

// --- In-Memory Caches & Queues ---
const dnsCache = new Map();      // domain -> MX records
const domainStatus = new Map();  // domain -> { isCatchAll: boolean, checked: boolean }
const domainQueues = new Map();  // domain -> Promise chain for rate limiting

// Configuration
const RATE_LIMIT_DELAY = 1000; // 1 second between checks to the same domain
const SMTP_TIMEOUT = 3000;     // 3 seconds timeout for socket operations

// --- Helper: Sleep ---
const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

// --- Helper: DNS MX Lookup ---
async function getMxRecords(domain) {
    if (dnsCache.has(domain)) return dnsCache.get(domain);
    try {
        const addresses = await dns.resolveMx(domain);
        if (!addresses || addresses.length === 0) throw new Error('No MX records');
        // Sort by priority
        addresses.sort((a, b) => a.priority - b.priority);
        dnsCache.set(domain, addresses);
        return addresses;
    } catch (error) {
        dnsCache.set(domain, null);
        return null;
    }
}

// --- Helper: SMTP Handshake ---
async function verifySMTP(email, mxRecord) {
    return new Promise((resolve) => {
        const socket = net.createConnection(25, mxRecord.exchange);
        let step = 0;
        let hasResolved = false;

        const cleanup = () => {
            if (!hasResolved) {
                hasResolved = true;
                socket.destroy();
            }
        };

        const finish = (result) => {
            if (!hasResolved) {
                hasResolved = true;
                resolve(result);
                socket.end();
                socket.destroy();
            }
        };

        socket.setTimeout(SMTP_TIMEOUT);

        socket.on('timeout', () => finish({ isValid: false, reason: 'SMTP Timeout' }));
        socket.on('error', (err) => finish({ isValid: false, reason: `SMTP Error: ${err.message}` }));

        socket.on('data', (data) => {
            const response = data.toString();
            // console.log(`[${email}] Server: ${response.trim()}`);

            // Simple state machine for SMTP handshake
            try {
                if (step === 0 && response.startsWith('220')) {
                    // Banner received, send HELO
                    socket.write(`HELO mail-sorter.local\r\n`);
                    step++;
                } else if (step === 1 && response.startsWith('250')) {
                    // HELO accepted, send MAIL FROM
                    socket.write(`MAIL FROM:<verify@mail-sorter.local>\r\n`);
                    step++;
                } else if (step === 2 && response.startsWith('250')) {
                    // MAIL FROM accepted, send RCPT TO
                    socket.write(`RCPT TO:<${email}>\r\n`);
                    step++;
                } else if (step === 3) {
                    // RCPT TO response
                    if (response.startsWith('250') || response.startsWith('251')) {
                        finish({ isValid: true, reason: 'Mailbox Exists' });
                    } else if (response.startsWith('550') || response.startsWith('551') || response.startsWith('553')) {
                        finish({ isValid: false, reason: 'User Not Found' });
                    } else {
                        // Greylisting, blocking, or other errors (4xx, 5xx other than user unknown)
                        finish({ isValid: false, reason: `SMTP Response: ${response.substring(0, 15)}...` });
                    }
                }
            } catch (e) {
                finish({ isValid: false, reason: 'Protocol Error' });
            }
        });
    });
}

// --- Helper: Rate Limited Executor ---
async function scheduleDomainTask(domain, taskFn) {
    if (!domainQueues.has(domain)) {
        domainQueues.set(domain, Promise.resolve());
    }

    const previousTask = domainQueues.get(domain);

    const nextTask = previousTask.then(async () => {
        await sleep(RATE_LIMIT_DELAY); // Enforce delay
        return taskFn();
    }).catch(() => {
        // If previous task failed, we still run this one, just need to catch the error to keep chain alive
        return taskFn();
    });

    domainQueues.set(domain, nextTask);
    return nextTask;
}

// --- Core Logic: Smart Verify ---
async function smartVerify(email) {
    const parts = email.split('@');
    if (parts.length !== 2) return { isValid: false, reason: 'Invalid Format' };
    const domain = parts[1].toLowerCase();

    // 1. Get MX Records
    const mxRecords = await getMxRecords(domain);
    if (!mxRecords) return { isValid: false, reason: 'No MX Records' };

    // 2. Check Catch-All Status (if not checked yet)
    if (!domainStatus.has(domain)) {
        // Probe a random address
        const randomUser = `probe_${Math.random().toString(36).substring(7)}`;
        const probeEmail = `${randomUser}@${domain}`;

        // Schedule the probe
        const probeResult = await scheduleDomainTask(domain, () => verifySMTP(probeEmail, mxRecords[0]));

        const isCatchAll = probeResult.isValid;
        domainStatus.set(domain, { isCatchAll, checked: true });
        console.log(`[${domain}] Catch-All Status: ${isCatchAll}`);
    }

    const status = domainStatus.get(domain);
    if (status.isCatchAll) {
        return { isValid: true, reason: 'Catch-All Domain (Risky)', isCatchAll: true };
    }

    // 3. Verify Actual User (Rate Limited)
    const result = await scheduleDomainTask(domain, () => verifySMTP(email, mxRecords[0]));
    return result;
}

app.post('/api/validate-batch', async (req, res) => {
    const { emails } = req.body;
    console.log(`Processing Smart Batch of ${emails.length} emails...`);

    // Process in parallel (but rate limited internally per domain)
    const results = await Promise.all(emails.map(async (email) => {
        try {
            const result = await smartVerify(email);
            return { email, ...result };
        } catch (e) {
            return { email, isValid: false, reason: 'Internal Error' };
        }
    }));

    res.json({ results });
});

app.listen(PORT, () => {
    console.log(`Smart SMTP Server running on http://localhost:${PORT}`);
});
