package client

import "core:encoding/json"
import "core:fmt"

EventType :: enum {
	DiscoverLocal       = 0,
	RequestHandshake    = 1,
	AcknowledgePublicIP = 2,
}

Message_Data :: union {
	Discovery_Result,
	Public_IP_Result,
	Handshake_Request
}

Discovery_Result :: struct {
	discovered: []string,
}

Public_IP_Result :: struct {
	ip: string,
}

Handshake_Request :: struct {
	endpoint: string,
}


Message :: struct {
	type: EventType,
	data: Message_Data,
}

Message_Header :: struct {
	type: EventType,
}

Server_Message :: struct {
	using _: Message,
}


Message_Handler :: #type proc(message: []byte, data_context: ^Session)

message_handlers: map[EventType]Message_Handler

make_handler :: proc($T: typeid, $handler: proc(_: T, _: ^Session)) -> Message_Handler {
	return proc(handler_data: []byte, ctx: ^Session) {
			payload: struct {
				data: T,
			}
			err := json.unmarshal(handler_data, &payload)
			if err != nil {
				fmt.printf("couldn't unmarshal %v: %v\n", typeid_of(T), err)
				return
			}


			// result, ok := payload.(T)
			// if !ok {
			// 	fmt.println(handler_data)
			// 	panic(fmt.tprintf("couldn't assert %v\n", typeid_of(T)))
			// }
			handler(payload.data, ctx)
		}
}


handle_discover :: proc(message: Discovery_Result, data_context: ^Session) {
	fmt.println("handle_discover")
	for addr in message.discovered {
		append(&data_context.addresses, addr)
	}

	fmt.println(message)
}

handle_public_ip_acknowledge :: proc(message: Public_IP_Result, data_context: ^Session) {
	data_context.public_ip = message.ip
}


initialize_handlers :: proc() {
	message_handlers = make(map[EventType]Message_Handler)
	message_handlers[EventType.DiscoverLocal] = make_handler(Discovery_Result, handle_discover)
	message_handlers[EventType.AcknowledgePublicIP] = make_handler(
		Public_IP_Result,
		handle_public_ip_acknowledge,
	)
}

get_handlers :: proc() -> map[EventType]Message_Handler {
	return message_handlers
}
