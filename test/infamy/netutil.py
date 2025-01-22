import socket

def tcp_port_is_open(host, port):
    try:
        ai = socket.getaddrinfo(host, port, 0, 0, socket.SOL_TCP)
        sock = socket.socket(ai[0][0], ai[0][1], 0)
        sock.connect(ai[0][4])
        sock.close()
        return True
    except Exception:
        return False
