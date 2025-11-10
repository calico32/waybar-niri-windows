sources := $(wildcard lib/*.go) $(wildcard lib/*.c) $(wildcard lib/*.h) $(wildcard log/*.go) $(wildcard main/*.go) $(wildcard niri/*.go) $(wildcard module/*.go)

waybar-niri-windows-debug.so: $(sources)
	go build -buildmode=c-shared -tags debug -o $@ ./lib

waybar-niri-windows.so: $(sources)
	go build -buildmode=c-shared -o $@ ./lib

waybar:
	waybar -c test/config.jsonc -s test/style.css

clean:
	rm -f waybar-niri-windows.so
	rm -f waybar-niri-windows-debug.so

.PHONY: waybar clean
