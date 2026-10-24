package transport

import "core:encoding/json"
import "core:fmt"
import "core:sys/posix"


Unix_Socket :: struct {
	fd: posix.FD,
}


connect_unix_socket :: proc(path: string) -> Unix_Socket {
	unix_sock_fd := posix.socket(posix.AF.UNIX, posix.Sock.SEQPACKET)

	addr: posix.sockaddr
	addr.sa_family = posix.sa_family_t.UNIX

	copy(addr.sa_data[:], fmt.tprintf("%s\x00", path))

	res := posix.bind(unix_sock_fd, &addr, size_of(addr))
	conn_err := posix.connect(unix_sock_fd, &addr, size_of(addr))
	fmt.println("connection error = ", conn_err)

	return Unix_Socket{fd = unix_sock_fd}
}


read_unix :: proc(fd: posix.FD, buf: []byte) -> (int, posix.Errno) {
	n := posix.read(fd, raw_data(buf[:]), len(buf))
	if n == -1 {
		return 0, posix.get_errno()
	}
	return n, nil
}


write_unix :: proc(socket: Unix_Socket, buffer: []byte) -> (int, posix.Errno) {
	written := posix.write(socket.fd, raw_data(buffer), len(buffer))
	err: posix.Errno
	if written == -1 {
		err = posix.get_errno()
		fmt.println("write unix is failed")
		fmt.println(err)
	}
	return written, err
}
