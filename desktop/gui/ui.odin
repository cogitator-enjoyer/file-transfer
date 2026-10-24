package gui

import "core:strings"
import "core:unicode/utf8"
import mu "vendor:microui"
import rl "vendor:raylib"


UI :: struct {
	ctx:   mu.Context,
	atlas: rl.Texture2D,
}


Text_Input :: struct {
	buf: [256]u8,
	len: int,
}

text_input_string :: proc(t: ^Text_Input) -> string {
	return string(t.buf[:t.len])
}

text_input_clear :: proc(t: ^Text_Input) {
	t.len = 0
}


ACCENT :: mu.Color{88, 132, 255, 255}
ACCENT_HOVER :: mu.Color{110, 150, 255, 255}
ACCENT_FOCUS :: mu.Color{70, 110, 230, 255}
TEXT_DIM :: mu.Color{130, 136, 150, 255}

apply_theme :: proc(ctx: ^mu.Context) {
	s := ctx.style
	s.padding = 8
	s.spacing = 6
	s.title_height = 28
	s.scrollbar_size = 10
	s.thumb_size = 8
	s.colors = {
		.TEXT         = {225, 228, 235, 255},
		.SELECTION_BG = {60, 90, 170, 255},
		.BORDER       = {20, 22, 28, 255},
		.WINDOW_BG    = {32, 34, 42, 255},
		.TITLE_BG     = {24, 26, 32, 255},
		.TITLE_TEXT   = {240, 242, 248, 255},
		.PANEL_BG     = {26, 28, 35, 255},
		.BUTTON       = {52, 56, 70, 255},
		.BUTTON_HOVER = {64, 69, 86, 255},
		.BUTTON_FOCUS = {76, 82, 102, 255},
		.BASE         = {22, 24, 30, 255},
		.BASE_HOVER   = {26, 28, 36, 255},
		.BASE_FOCUS   = {30, 33, 42, 255},
		.SCROLL_BASE  = {28, 30, 38, 255},
		.SCROLL_THUMB = {70, 75, 92, 255},
	}
}


init :: proc(ui: ^UI) {
	mu.init(&ui.ctx, set_clipboard, get_clipboard)
	ui.ctx.text_width = mu.default_atlas_text_width
	ui.ctx.text_height = mu.default_atlas_text_height
	apply_theme(&ui.ctx)


	pixels := make([][4]u8, mu.DEFAULT_ATLAS_WIDTH * mu.DEFAULT_ATLAS_HEIGHT)
	defer delete(pixels)
	for alpha, i in mu.default_atlas_alpha {
		pixels[i] = {255, 255, 255, alpha}
	}
	image := rl.Image {
		data    = raw_data(pixels),
		width   = mu.DEFAULT_ATLAS_WIDTH,
		height  = mu.DEFAULT_ATLAS_HEIGHT,
		mipmaps = 1,
		format  = .UNCOMPRESSED_R8G8B8A8,
	}
	ui.atlas = rl.LoadTextureFromImage(image)
}

shutdown :: proc(ui: ^UI) {
	rl.UnloadTexture(ui.atlas)
}

begin_frame :: proc(ui: ^UI) {
	forward_input(&ui.ctx)
	mu.begin(&ui.ctx)
}


end_frame :: proc(ui: ^UI) {
	mu.end(&ui.ctx)
	render(ui)
}


window :: proc(
	ui: ^UI,
	title: string,
	rect: mu.Rect,
	opt := mu.Options{.NO_RESIZE, .NO_CLOSE},
) -> bool {


	if cnt := mu.get_container(&ui.ctx, title, opt); cnt != nil {
		cnt.rect = rect
	}
	return mu.begin_window(&ui.ctx, title, rect, opt)
}

end_window :: proc(ui: ^UI) {
	mu.end_window(&ui.ctx)
}


button :: proc(ui: ^UI, label: string) -> bool {
	return .SUBMIT in mu.button(&ui.ctx, label)
}


primary_button :: proc(ui: ^UI, label: string) -> bool {
	colors := &ui.ctx.style.colors
	saved := [3]mu.Color{colors[.BUTTON], colors[.BUTTON_HOVER], colors[.BUTTON_FOCUS]}
	colors[.BUTTON] = ACCENT
	colors[.BUTTON_HOVER] = ACCENT_HOVER
	colors[.BUTTON_FOCUS] = ACCENT_FOCUS
	defer {
		colors[.BUTTON] = saved[0]
		colors[.BUTTON_HOVER] = saved[1]
		colors[.BUTTON_FOCUS] = saved[2]
	}
	return button(ui, label)
}


disabled_button :: proc(ui: ^UI, label: string) {
	ctx := &ui.ctx
	r := mu.layout_next(ctx)
	mu.draw_rect(ctx, r, ctx.style.colors[.BASE])
	draw_centered_text(ctx, label, r, TEXT_DIM)
}


input :: proc(ui: ^UI, field: ^Text_Input, placeholder := "") -> (submitted: bool) {
	ctx := &ui.ctx
	res := mu.textbox(ctx, field.buf[:], &field.len)
	id := ctx.last_id
	r := ctx.last_rect

	if field.len == 0 && ctx.focus_id != id && placeholder != "" {
		pos := mu.Vec2{r.x + ctx.style.padding, r.y + (r.h - ctx.text_height(ctx.style.font)) / 2}
		mu.draw_text(ctx, ctx.style.font, placeholder, pos, TEXT_DIM)
	}


	if ctx.focus_id == id {
		draw_outline(ctx, r, ACCENT)
	}

	if .SUBMIT in res {
		mu.set_focus(ctx, 0)
		return true
	}
	return false
}


labeled_input :: proc(ui: ^UI, label: string, field: ^Text_Input, placeholder := "") -> bool {
	mu.layout_row(&ui.ctx, {-1}, 16)
	text_dim(ui, label)
	mu.layout_row(&ui.ctx, {-1}, 30)
	return input(ui, field, placeholder)
}

section :: proc(ui: ^UI, title: string) {
	ctx := &ui.ctx
	mu.layout_row(ctx, {-1}, 22)
	r := mu.layout_next(ctx)
	mu.draw_text(ctx, ctx.style.font, title, {r.x, r.y + 2}, ctx.style.colors[.TITLE_TEXT])
	mu.draw_rect(ctx, {r.x, r.y + r.h - 1, r.w, 1}, ctx.style.colors[.BUTTON])
}

text :: proc(ui: ^UI, str: string) {
	mu.label(&ui.ctx, str)
}

text_dim :: proc(ui: ^UI, str: string) {
	ctx := &ui.ctx
	r := mu.layout_next(ctx)
	mu.draw_text(
		ctx,
		ctx.style.font,
		str,
		{r.x, r.y + (r.h - ctx.text_height(ctx.style.font)) / 2},
		TEXT_DIM,
	)
}


row :: proc(ui: ^UI, items: i32, height: i32 = 30) {
	ctx := &ui.ctx
	items := clamp(items, 1, mu.MAX_WIDTHS)
	body_w := mu.get_layout(ctx).body.w
	w := (body_w - ctx.style.spacing * (items - 1)) / items
	widths: [mu.MAX_WIDTHS]i32
	for i in 0 ..< items {
		widths[i] = w
	}
	mu.layout_row(ctx, widths[:items], height)
}


row_widths :: proc(ui: ^UI, widths: []i32, height: i32 = 30) {
	mu.layout_row(&ui.ctx, widths, height)
}


begin_list :: proc(ui: ^UI, name: string, height: i32) {
	mu.layout_row(&ui.ctx, {-1}, height)
	mu.begin_panel(&ui.ctx, name)
}

end_list :: proc(ui: ^UI) {
	mu.end_panel(&ui.ctx)
}


draw_outline :: proc(ctx: ^mu.Context, r: mu.Rect, color: mu.Color) {
	mu.draw_rect(ctx, {r.x, r.y, r.w, 1}, color)
	mu.draw_rect(ctx, {r.x, r.y + r.h - 1, r.w, 1}, color)
	mu.draw_rect(ctx, {r.x, r.y, 1, r.h}, color)
	mu.draw_rect(ctx, {r.x + r.w - 1, r.y, 1, r.h}, color)
}

draw_centered_text :: proc(ctx: ^mu.Context, str: string, r: mu.Rect, color: mu.Color) {
	font := ctx.style.font
	tw := ctx.text_width(font, str)
	th := ctx.text_height(font)
	mu.draw_text(ctx, font, str, {r.x + (r.w - tw) / 2, r.y + (r.h - th) / 2}, color)
}

set_clipboard :: proc(user_data: rawptr, text: string) -> bool {
	rl.SetClipboardText(strings.clone_to_cstring(text, context.temp_allocator))
	return true
}

get_clipboard :: proc(user_data: rawptr) -> (string, bool) {
	cstr := rl.GetClipboardText()
	if cstr == nil {
		return "", false
	}
	return string(cstr), true
}

KEY_MAP := [?]struct {
	rl_key: rl.KeyboardKey,
	mu_key: mu.Key,
} {
	{.LEFT_SHIFT, .SHIFT},
	{.RIGHT_SHIFT, .SHIFT},
	{.LEFT_CONTROL, .CTRL},
	{.RIGHT_CONTROL, .CTRL},
	{.LEFT_ALT, .ALT},
	{.RIGHT_ALT, .ALT},
	{.BACKSPACE, .BACKSPACE},
	{.DELETE, .DELETE},
	{.ENTER, .RETURN},
	{.KP_ENTER, .RETURN},
	{.LEFT, .LEFT},
	{.RIGHT, .RIGHT},
	{.HOME, .HOME},
	{.END, .END},
	{.A, .A},
	{.X, .X},
	{.C, .C},
	{.V, .V},
}

MOUSE_MAP := [?]struct {
	rl_btn: rl.MouseButton,
	mu_btn: mu.Mouse,
}{{.LEFT, .LEFT}, {.RIGHT, .RIGHT}, {.MIDDLE, .MIDDLE}}

forward_input :: proc(ctx: ^mu.Context) {
	mx, my := rl.GetMouseX(), rl.GetMouseY()
	mu.input_mouse_move(ctx, mx, my)

	wheel := rl.GetMouseWheelMoveV()
	mu.input_scroll(ctx, i32(wheel.x * -30), i32(wheel.y * -30))

	for m in MOUSE_MAP {
		if rl.IsMouseButtonPressed(m.rl_btn) do mu.input_mouse_down(ctx, mx, my, m.mu_btn)
		if rl.IsMouseButtonReleased(m.rl_btn) do mu.input_mouse_up(ctx, mx, my, m.mu_btn)
	}

	for k in KEY_MAP {

		if rl.IsKeyPressed(k.rl_key) || rl.IsKeyPressedRepeat(k.rl_key) do mu.input_key_down(ctx, k.mu_key)
		if rl.IsKeyReleased(k.rl_key) do mu.input_key_up(ctx, k.mu_key)
	}

	text_buf: [64]u8
	n := 0
	for ch := rl.GetCharPressed(); ch != 0; ch = rl.GetCharPressed() {
		bytes, w := utf8.encode_rune(ch)
		if n + w > len(text_buf) do break
		copy(text_buf[n:], bytes[:w])
		n += w
	}
	if n > 0 {
		mu.input_text(ctx, string(text_buf[:n]))
	}
}

to_rl_color :: #force_inline proc(c: mu.Color) -> rl.Color {
	return {c.r, c.g, c.b, c.a}
}

atlas_rect :: #force_inline proc(r: mu.Rect) -> rl.Rectangle {
	return {f32(r.x), f32(r.y), f32(r.w), f32(r.h)}
}

render :: proc(ui: ^UI) {
	ctx := &ui.ctx
	rl.BeginScissorMode(0, 0, rl.GetScreenWidth(), rl.GetScreenHeight())
	defer rl.EndScissorMode()

	cmd: ^mu.Command
	for variant in mu.next_command_iterator(ctx, &cmd) {
		switch c in variant {
		case ^mu.Command_Text:
			pos := rl.Vector2{f32(c.pos.x), f32(c.pos.y)}
			for ch in c.str {
				if ch & 0xc0 == 0x80 do continue
				r := mu.default_atlas[mu.DEFAULT_ATLAS_FONT + min(int(ch), 127)]
				rl.DrawTextureRec(ui.atlas, atlas_rect(r), pos, to_rl_color(c.color))
				pos.x += f32(r.w)
			}
		case ^mu.Command_Rect:
			rl.DrawRectangle(c.rect.x, c.rect.y, c.rect.w, c.rect.h, to_rl_color(c.color))
		case ^mu.Command_Icon:
			r := mu.default_atlas[c.id]
			x := c.rect.x + (c.rect.w - r.w) / 2
			y := c.rect.y + (c.rect.h - r.h) / 2
			rl.DrawTextureRec(ui.atlas, atlas_rect(r), {f32(x), f32(y)}, to_rl_color(c.color))
		case ^mu.Command_Clip:
			rl.EndScissorMode()
			rl.BeginScissorMode(c.rect.x, c.rect.y, c.rect.w, c.rect.h)
		case ^mu.Command_Jump:
			unreachable()
		}
	}
}
