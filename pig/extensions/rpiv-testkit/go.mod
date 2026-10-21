module github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-testkit

go 1.26

require (
	github.com/MichaelKinsy/PiG/extensions/sdk v0.5.0
	github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host v0.0.0
)

replace github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host => ../rpiv-host

replace github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui => ../rpiv-tui
