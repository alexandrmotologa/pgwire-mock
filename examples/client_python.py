#!/usr/bin/env python3
"""
Python verification client for PGWire-Mock.
Speaks PostgreSQL Frontend/Backend Protocol 3.0 directly over standard sockets.
"""

import os
import socket
import struct
import sys

HOST = os.getenv("PGHOST", "127.0.0.1")
PORT = int(os.getenv("PGPORT", "5432"))

def main():
    print(f"[Python Client] Connecting to PGWire-Mock at {HOST}:{PORT}...")
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.connect((HOST, PORT))
    except Exception as e:
        print(f"[Python Client] Connection failed: {e}")
        sys.exit(1)

    print("[Python Client] Connected. Sending SSLRequest negotiation packet...")
    # SSLRequest: int32(8), int32(80877103)
    ssl_req = struct.pack("!II", 8, 80877103)
    s.sendall(ssl_req)

    ssl_resp = s.recv(1)
    print(f"[Python Client] SSL response: '{ssl_resp.decode('ascii')}' (N = plaintext fallback)")

    # Send StartupMessage
    params = b"user\x00postgres\x00database\x00testdb\x00\x00"
    startup_len = 4 + 4 + len(params)
    startup_msg = struct.pack("!II", startup_len, 196608) + params
    s.sendall(startup_msg)

    # Read until ReadyForQuery ('Z')
    authenticated = False
    ready = False

    while not ready:
        msg_type = s.recv(1)
        if not msg_type:
            break
        raw_len = s.recv(4)
        if len(raw_len) < 4:
            break
        msg_len = struct.unpack("!I", raw_len)[0]
        payload = s.recv(msg_len - 4)

        if msg_type == b'R':
            print("[Python Client] AuthenticationOk received.")
            authenticated = True
        elif msg_type == b'Z':
            print("[Python Client] ReadyForQuery received (Server is IDLE).")
            ready = True

    if not authenticated or not ready:
        print("[Python Client] Handshake failed.")
        sys.exit(1)

    # Send Simple Query
    query = "SELECT id, email, full_name, role FROM users WHERE id = '1'"
    print(f"[Python Client] Sending query: \"{query}\"")
    q_bytes = query.encode("utf-8") + b"\x00"
    q_packet = b"Q" + struct.pack("!I", 4 + len(q_bytes)) + q_bytes
    s.sendall(q_packet)

    # Read response
    while True:
        msg_type = s.recv(1)
        if not msg_type:
            break
        raw_len = s.recv(4)
        if len(raw_len) < 4:
            break
        msg_len = struct.unpack("!I", raw_len)[0]
        payload = s.recv(msg_len - 4)

        if msg_type == b'T':
            print("[Python Client] RowDescription received (columns metadata).")
        elif msg_type == b'D':
            # Parse DataRow column strings
            num_cols = struct.unpack("!H", payload[:2])[0]
            offset = 2
            row_vals = []
            for _ in range(num_cols):
                col_len = struct.unpack("!i", payload[offset:offset+4])[0]
                offset += 4
                if col_len == -1:
                    row_vals.append("NULL")
                else:
                    val = payload[offset:offset+col_len].decode("utf-8")
                    row_vals.append(val)
                    offset += col_len
            print(f"[Python Client] DataRow: {row_vals}")
        elif msg_type == b'C':
            tag = payload[:-1].decode("utf-8")
            print(f"[Python Client] CommandComplete: \"{tag}\"")
        elif msg_type == b'Z':
            print("[Python Client] ReadyForQuery received. Verification successful!")
            break

    s.close()
    print("[Python Client] Done.")

if __name__ == "__main__":
    main()
