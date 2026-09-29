import socket

def tcp_port_is_open(host, port, timeout=3):
    try:
        with socket.create_connection((host, port), timeout=timeout):
            return True
    except (socket.timeout, OSError):
        return False

def tcp_read(host, port, timeout=10):
    """Read everything the server sends until it closes the connection"""
    data = b""
    with socket.create_connection((host, port), timeout=timeout) as sock:
        while chunk := sock.recv(1024):
            data += chunk
    return data.decode(errors="replace")
