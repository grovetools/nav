module github.com/mattsolo1/grove-tmux

go 1.24

toolchain go1.24.4

require (
	github.com/mattsolo1/grove-core v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.9.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
)

replace github.com/mattsolo1/grove-core => ../grove-core
