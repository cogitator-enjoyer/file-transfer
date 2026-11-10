package client

import "core:fmt"
import "../network"
import "core:encoding/json"


// @private
// Pending_Messages :: struct {
// 	rwmutex: sync.RW_Mutex
// }

Session :: struct {
	addresses: [dynamic]string,
	public_ip: string,
}

Read_Error :: union #shared_nil {
	json.Unmarshal_Error,
	network.Recv_Error,
}

read :: proc(msg: ^Message, conn: ^network.Server_Conn) -> Read_Error {
	buffer: [1024 * 4]byte
	r := network.read(buffer[:], conn) or_return
	json.unmarshal(buffer[:r], msg) or_return
	return nil
}

Write_Error :: union #shared_nil {
	json.Marshal_Error,
	network.Send_Error,
}

write :: proc(msg: Message, conn: ^network.Server_Conn) -> Write_Error {
	result_json := json.marshal(msg) or_return
	defer delete(result_json)
	network.send(result_json, conn) or_return
	fmt.println(msg)
	return nil
}

send :: proc(
	type: Command,
	data: Message_Data = {},
	conn: ^network.Server_Conn,
) -> Write_Error {
	err := write(new_message(type, data), conn)
	if err != nil {
		panic(fmt.tprintf("%v\n", err))
	}
	return err
}
