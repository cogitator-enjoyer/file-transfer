package main

import "client"
import "core:c"
import "core:encoding/json"
import "core:fmt"
import "core:mem"
import "core:sys/posix"
import "core:thread"
import tfdialog "external-libs/tinyfiledialogs"
import "gui"
import client_net "network"
import "network/transport"
import rl "vendor:raylib"


Callback_Context :: struct {
	session: ^client.Session,
	conn:    ^client_net.Server_Conn,
}


listenServer :: proc(task: thread.Task) {
	user_data := cast(^struct {
		conn:         ^client_net.Server_Conn,
		data_context: ^client.Session,
		pool:         ^thread.Pool,
	})task.data
	server := user_data.conn
	pool := user_data.pool

	for {
		buffer: [1024]byte
		n, _ := transport.read_unix(server.socket.fd, buffer[:])
		if n > 0 {
			fmt.println("readed", n, "bytes")
			fmt.println(string(buffer[:n]))
		}

		server_message: client.Server_Message
		err := json.unmarshal(buffer[:], &server_message)
		if err != nil {
			fmt.printf("couldn't unmarshal server message: %v\n", err)
			continue
		}
		fmt.println("server_message", server_message)
		fmt.println("server_message.type", server_message.type)
		handler, ok := client.get_handlers()[server_message.type]
		if !ok {
			fmt.printf("handler %v is not registered\n", server_message.type)
			fmt.printf("%d\n", server_message.type)
			continue
		}

		handler_data_context := new(struct {
				handler_procedure: client.Message_Handler,
				handler_data:      client.Message_Data,
				ctx:               ^client.Session,
			})

		handler_data_context.ctx = user_data.data_context
		handler_data_context.handler_data = server_message.data
		handler_data_context.handler_procedure = handler


		thread.pool_add_task(
			pool = pool,
			allocator = context.allocator,
			procedure = thread_handle_server_message,
			data = handler_data_context,
		)
	}
}


thread_handle_server_message :: proc(task: thread.Task) {
	context_data := cast(^struct {
		handler_procedure: client.Message_Handler,
		handler_data:      client.Message_Data,
		ctx:               ^client.Session,
	})task.data
	context_data.handler_procedure(context_data.handler_data, context_data.ctx)
	free(context_data)
}


main :: proc() {
	m_n := rl.GetCurrentMonitor()
	monitor_height: c.int = 800
	monitor_width: c.int = 600

	rl.SetConfigFlags({.WINDOW_RESIZABLE, .VSYNC_HINT})
	rl.InitWindow(monitor_width, monitor_height, "file transfer")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	ui := new(gui.UI)
	defer free(ui)
	gui.init(ui)
	defer gui.shutdown(ui)

	path := "/tmp/unix.sock"
	socket := transport.connect_unix_socket(path)
	server_conn := client_net.Server_Conn {
		socket = socket,
	}

	pool := thread.Pool{}
	data := new(struct {
			conn:         ^client_net.Server_Conn,
			data_context: ^client.Session,
			pool:         ^thread.Pool,
		})
	data_context := client.Session {
		addresses = make([dynamic]string),
	}
	data.conn = &server_conn
	data.data_context = &data_context
	data.pool = &pool

	thread.pool_init(pool = &pool, allocator = context.allocator, thread_count = 3)
	thread.pool_add_task(
		pool = &pool,
		allocator = context.allocator,
		procedure = listenServer,
		data = data,
	)
	thread.pool_start(&pool)
	defer posix.close(socket.fd)

	client.initialize_handlers()


	ui_state := Ui_State {
		selected = -1,
	}

	for !rl.WindowShouldClose() {
		gui.begin_frame(ui)
		draw_main_screen(ui, &ui_state, &data_context, &server_conn)

		rl.BeginDrawing()
		rl.ClearBackground({18, 19, 24, 255})
		gui.end_frame(ui)
		free_all(context.temp_allocator)
		rl.EndDrawing()
	}
}

Ui_State :: struct {
	peer_address: gui.Text_Input,
	file_path:    gui.Text_Input,
	selected:     int,
	status:       string,
	status_buf:   [256]u8,
}

draw_main_screen :: proc(
	ui: ^gui.UI,
	state: ^Ui_State,
	session: ^client.Session,
	conn: ^client_net.Server_Conn,
) {
	w, h := rl.GetScreenWidth(), rl.GetScreenHeight()
	if !gui.window(ui, "file transfer", {0, 0, i32(w), i32(h)}) do return
	defer gui.end_window(ui)

	gui.section(ui, "Peers")
	gui.row(ui, 2)
	if gui.primary_button(ui, "Discover") {
		client.write(client.Message{type = .DiscoverLocal}, conn)
		state.status = "discovering..."
	}
	if gui.button(ui, "Clear") {
		clear(&session.addresses)
		state.selected = -1
	}

	gui.begin_list(ui, "peers", 200)
	if len(session.addresses) == 0 {
		gui.row(ui, 1, 24)
		gui.text_dim(ui, "no peers yet - press Discover")
	}
	for addr, i in session.addresses {
		gui.row(ui, 1, 26)
		label := fmt.tprintf("> %s", addr) if i == state.selected else addr
		if gui.button(ui, label) {
			state.selected = i
			copy(state.peer_address.buf[:], addr)
			state.peer_address.len = min(len(addr), len(state.peer_address.buf))
		}
	}
	gui.end_list(ui)

	gui.section(ui, "Connect")
	submitted := gui.labeled_input(ui, "Peer address", &state.peer_address, "192.168.0.10:9000")
	gui.row_widths(ui, {-90, -1})
	gui.text_dim(ui, "")
	if gui.primary_button(ui, "Connect") || submitted {
		state.status = fmt.bprintf(
			state.status_buf[:],
			"connecting to %s",
			gui.text_input_string(&state.peer_address),
		)
	}

	gui.section(ui, "Send file")
	gui.labeled_input(ui, "File path", &state.file_path, "/home/user/file.txt")
	gui.row(ui, 2)
	if gui.button(ui, "Browse...") {
		path := tfdialog.openFileDialog("Browse", "", 0, nil, "", 1)
		fmt.println("path is", path)
	}
	if gui.text_input_string(&state.file_path) == "" {
		gui.disabled_button(ui, "Send")
	} else if gui.primary_button(ui, "Send") {


		state.status = "sending..."
	}

	if state.status != "" {
		gui.row(ui, 1, 24)
		gui.text_dim(ui, state.status)
	}
}
