package conn

import "core:encoding/json"
import "core:fmt"
import "core:sys/posix"
import "transport"

Send_Error :: union #shared_nil {
	posix.Errno,
}

Recv_Error :: union #shared_nil {
	posix.Errno,
}

Server_Conn :: struct {
	socket: transport.Unix_Socket,
}

init_unix_connection :: proc(path: string) -> Server_Conn {
	socket := transport.connect_unix_socket(path)
	return Server_Conn{socket = socket}
}


read :: proc(buf: []byte, conn: ^Server_Conn) -> (int, Recv_Error) {
	n, err := transport.read_unix(conn.socket.fd, buf)
	return n, err
}

send :: proc(msg_buf: []byte, connection: ^Server_Conn) -> Send_Error {
	_, err := transport.write_unix(connection.socket, msg_buf)
	return err
}
