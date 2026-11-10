package client

import "core:encoding/json"
import "core:fmt"
import "core:sync"

current_event_id: u32 = 1


Command :: enum {
	DiscoverLocal = 1,
	RequestHandshake,
	AcknowledgePublicIP,
}

Message_Data :: union {
	Handshake_Request,
}

Response_Data :: union {
	Discovery_Result,
	Public_IP_Result,
}


Message :: struct {
	id:   u32,
	type: Command,
	data: Message_Data,
}

Event_Header :: struct {
	type: Event_Type,
}

Event :: struct {
	reply_to: u32,
	type:     Event_Type,
	data:     Response_Data,
}

Event_Type :: enum {
	Ev_Discover_Result = 1,
	Ev_Handshake_Result,
	Ev_Incoming_Handshake,
	Ev_Error,
	Ev_Public_IP,
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


Message_Handler :: #type proc(message: []byte, data_context: ^Session)

event_handlers: map[Event_Type]Message_Handler

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
	event_handlers = make(map[Event_Type]Message_Handler)
	event_handlers[Event_Type.Ev_Discover_Result] = make_handler(Discovery_Result, handle_discover)
	event_handlers[Event_Type.Ev_Public_IP] = make_handler(
		Public_IP_Result,
		handle_public_ip_acknowledge,
	)
}

get_handlers :: proc() -> map[Event_Type]Message_Handler {
	return event_handlers
}

new_message :: proc(type: Command, data: Message_Data = {}) -> Message {
	return Message{type = type, data = data, id = get_id()}
}

get_id :: proc() -> u32 {
	next := sync.atomic_add(&current_event_id, current_event_id + 1)
	return next
}
