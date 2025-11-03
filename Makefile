waybar-niri-windows.so: $(wildcard lib/*.go) $(wildcard lib/*.c) $(wildcard lib/*.h) $(wildcard main/*.go) $(wildcard niri/*.go) $(wildcard module/*.go)
	go build -buildmode=c-shared -o $@ ./main

waybar:
	waybar -c test/config.jsonc -s test/style.css

clean:
	rm -f waybar-niri-windows.so

.PHONY: waybar clean
