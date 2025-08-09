INTERNAL=internal/chat/processor.go internal/config/config.go internal/corrade/client.go internal/macros/manager.go internal/types/types.go internal/web/interface.go


ALL: main.go $(INTERNAL)
	go build -o slbot main.go
