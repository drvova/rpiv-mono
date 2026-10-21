module github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host

go 1.26

require (
	github.com/MichaelKinsy/PiG/extensions/sdk v0.5.0
	github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui v0.0.0
)

require github.com/rivo/uniseg v0.4.7 // indirect

replace github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui => ../rpiv-tui
