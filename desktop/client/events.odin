package client

import "core:fmt"

EventType :: enum {
	DiscoverLocal = 0,
}

Message_Data :: union {
	Discovery_Result,
}

Discovery_Result :: struct {
	discovered: []string,
}


Message :: struct {
	type: EventType,
	data: Message_Data,
}

Server_Message :: struct {
	using _: Message,
}


Message_Handler :: #type proc(message: Message_Data, data_context: ^Session)

message_handlers: map[EventType]Message_Handler

make_handler :: proc($T: typeid, $handler: proc(_: T, _: ^Session)) -> Message_Handler {
	return proc(handler_data: Message_Data, ctx: ^Session) {
			result, ok := handler_data.(T)
			if !ok {
				panic(fmt.tprintf("couldn't assert %v\n", typeid_of(T)))
			}
			handler(result, ctx)
		}
}


handle_discover :: proc(message: Discovery_Result, data_context: ^Session) {
	fmt.println("handle_discover")
	for addr in message.discovered {
		append(&data_context.addresses, addr)
	}

	fmt.println(message)
}


initialize_handlers :: proc() {
	message_handlers = make(map[EventType]Message_Handler)
	message_handlers[EventType.DiscoverLocal] = make_handler(Discovery_Result, handle_discover)
}

get_handlers :: proc() -> map[EventType]Message_Handler {
	return message_handlers
}
