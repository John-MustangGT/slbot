# Second Life Bot Project Structure

```
slbot/
├── go.mod
├── main.go
├── bot_config.xml
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── types/
│   │   └── types.go
│   ├── corrade/
│   │   └── client.go
│   ├── chat/
│   │   └── processor.go
│   └── web/
│       └── interface.go
└── web/
    ├── templates/
    │   └── dashboard.html
    └── static/
        ├── css/
        │   └── dashboard.css
        └── js/
            └── dashboard.js
```

## Setup Instructions

1. **Initialize the project:**
   ```bash
   mkdir slbot
   cd slbot
   go mod init slbot
   go get github.com/gorilla/mux
   go get github.com/gorilla/websocket
   ```

2. **Create directory structure:**
   ```bash
   mkdir -p internal/config internal/types internal/corrade internal/chat internal/web
   mkdir -p web/templates web/static/css web/static/js
   ```

3. **Create the configuration file:**
   Copy the `bot_config.xml` content to the root directory.

4. **Copy all the Go files:**
   - `main.go` to root
   - Individual module files to their respective directories
   - HTML, CSS, and JS files to web directories

5. **Run the bot:**
   ```bash
   go run main.go
   ```

## Key Features

### Modular Architecture
- **config**: Configuration loading and management
- **types**: Shared data structures
- **corrade**: All Corrade API interactions
- **chat**: Chat processing and AI responses  
- **web**: Web interface and HTTP handlers

### Web Interface
- Real-time dashboard at `http://localhost:8081`
- Movement controls (teleport, walk, sit, stand)
- Activity logs with filtering
- Auto-refresh functionality
- Responsive design for mobile/desktop

### Chat Processing
- Context-aware AI responses using Llama
- Movement command parsing
- Following behavior with proximity detection
- Configurable prompts and responses

### Corrade Integration
- Full Second Life bot control
- Event polling for chat messages
- Position tracking and movement
- Status monitoring

## Configuration

Edit `bot_config.xml` to customize:
- Corrade connection settings
- Llama/Ollama configuration  
- Bot behavior parameters
- AI prompts and responses
- Web interface port

## Dependencies

- **Gorilla Mux**: HTTP routing
- **Gorilla WebSocket**: Future WebSocket support
- **Ollama**: Local LLM server
- **Corrade**: Second Life bot framework
