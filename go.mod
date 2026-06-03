module discord-szx

go 1.26.3

require (
	gioui.org v0.9.0
	github.com/minio/selfupdate v0.6.0
	golang.org/x/sys v0.44.0
)

require (
	aead.dev/minisign v0.2.0 // indirect
	gioui.org/shader v1.0.8 // indirect
	github.com/go-text/typesetting v0.3.0 // indirect
	golang.org/x/crypto v0.0.0-20211209193657-4570a0811e8b // indirect
	golang.org/x/exp/shiny v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/image v0.26.0 // indirect
	golang.org/x/text v0.24.0 // indirect
)

// Use a vendored copy of Gio patched to center the window on its first
// windowed placement (see third_party/gioui.org/app/os_windows.go). This
// eliminates the startup jump where the window briefly appears at the OS
// default top-left position before the app re-centers it.
replace gioui.org => ./third_party/gioui.org
