/**
 * Client verification script in Node.js for PGWire-Mock.
 * Demonstrates connecting and querying using native TCP sockets and PostgreSQL v3.0 protocol framing.
 */

const net = require('net');

const PORT = parseInt(process.env.PGPORT || '5432', 10);
const HOST = process.env.PGHOST || '127.0.0.1';

console.log(`[Node Client] Connecting to PGWire-Mock at ${HOST}:${PORT}...`);

const socket = net.createConnection({ host: HOST, port: PORT }, () => {
  console.log('[Node Client] Connected to socket. Sending SSL negotiation request...');
  
  // SSLRequest packet: length 8, code 80877103
  const sslBuf = Buffer.alloc(8);
  sslBuf.writeUInt32BE(8, 0);
  sslBuf.writeUInt32BE(80877103, 4);
  socket.write(sslBuf);
});

let state = 'SSL';

socket.on('data', (chunk) => {
  if (state === 'SSL') {
    const reply = chunk.toString('ascii');
    console.log(`[Node Client] Received SSL response: '${reply[0]}' (N = unencrypted fallback)`);
    state = 'STARTUP';

    // Send StartupMessage: length + version (196608) + user\0postgres\0database\0testdb\0\0
    const body = Buffer.from('user\0postgres\0database\0testdb\0client_encoding\0UTF8\0\0', 'utf8');
    const startupBuf = Buffer.alloc(4 + 4 + body.length);
    startupBuf.writeUInt32BE(startupBuf.length, 0);
    startupBuf.writeUInt32BE(196608, 4); // Protocol 3.0
    body.copy(startupBuf, 8);
    socket.write(startupBuf);
    return;
  }

  if (state === 'STARTUP') {
    // Process backend handshake responses (AuthOK, ParameterStatus, ReadyForQuery)
    const msgType = String.fromCharCode(chunk[0]);
    if (msgType === 'R') {
      console.log('[Node Client] AuthenticationOk received.');
    }
    
    // Check if chunk ends with ReadyForQuery ('Z')
    const lastByte = chunk[chunk.length - 1];
    const zIdx = chunk.lastIndexOf(0x5a); // 'Z'
    if (zIdx !== -1) {
      console.log('[Node Client] ReadyForQuery received! Server is idle and ready.');
      state = 'QUERY';
      sendQuery('SELECT id, title, price_cents, stock_count FROM products WHERE active = true ORDER BY id ASC');
    }
    return;
  }

  if (state === 'QUERY') {
    // Process query results
    let offset = 0;
    while (offset < chunk.length) {
      const type = String.fromCharCode(chunk[offset]);
      if (offset + 5 > chunk.length) break;
      const len = chunk.readUInt32BE(offset + 1);

      if (type === 'T') {
        console.log('[Node Client] RowDescription received (columns metadata).');
      } else if (type === 'D') {
        console.log('[Node Client] DataRow packet received.');
      } else if (type === 'C') {
        const tag = chunk.slice(offset + 5, offset + 1 + len - 1).toString('utf8');
        console.log(`[Node Client] CommandComplete received: "${tag}"`);
      } else if (type === 'Z') {
        console.log('[Node Client] ReadyForQuery received. Query execution completed successfully!');
        socket.end();
        process.exit(0);
      }
      offset += 1 + len;
    }
  }
});

function sendQuery(sql) {
  console.log(`[Node Client] Sending Simple Query: "${sql}"`);
  const sqlBuf = Buffer.from(sql + '\0', 'utf8');
  const packet = Buffer.alloc(1 + 4 + sqlBuf.length);
  packet[0] = 0x51; // 'Q'
  packet.writeUInt32BE(4 + sqlBuf.length, 1);
  sqlBuf.copy(packet, 5);
  socket.write(packet);
}

socket.on('error', (err) => {
  console.error(`[Node Client] Socket error:`, err);
  process.exit(1);
});

socket.on('close', () => {
  console.log('[Node Client] Connection closed.');
});
