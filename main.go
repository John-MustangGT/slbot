package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"slbot/internal/chat"
	"slbot/internal/config"
	"slbot/internal/corrade"
	"slbot/internal/web"
)

func main() {
	// Load configuration
	configPath := "bot_config.xml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Corrade client
	corradeClient := corrade.NewClient(cfg.Corrade)

	// Set bot name for position tracking and avatar filtering
	corradeClient.SetBotName(cfg.Bot.Name)

	// Initialize chat processor
	chatProcessor := chat.NewProcessor(cfg, corradeClient)

	// Initialize web interface
	webInterface := web.NewInterface(cfg, corradeClient, chatProcessor)

	// Test connections
	log.Println("Testing Corrade connection...")
	if err := corradeClient.TestConnection(); err != nil {
		log.Fatalf("Failed to connect to Corrade: %v", err)
	}

	log.Println("Testing Llama connection...")
	if err := chatProcessor.TestConnection(); err != nil {
		log.Fatalf("Failed to connect to Llama: %v", err)
	}

	// Start services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start web interface (this will also start avatar tracking)
	go func() {
		if err := webInterface.Start(ctx); err != nil {
			log.Printf("Web interface error: %v", err)
		}
	}()

	// Start chat processing
	go func() {
		if err := chatProcessor.Start(ctx); err != nil {
			log.Printf("Chat processor error: %v", err)
		}
	}()

	// Setup Corrade notifications for chat events
	callbackURL := fmt.Sprintf("http://localhost:%d/corrade/notifications", cfg.Bot.WebPort)
	
	// Setup chat notifications
	if err := corradeClient.SetupNotification("chat", callbackURL); err != nil {
		log.Printf("Failed to setup chat notification: %v", err)
	}
	
	// Setup instant message notifications
	if err := corradeClient.SetupNotification("instantmessage", callbackURL); err != nil {
		log.Printf("Failed to setup IM notification: %v", err)
	}

	// Announce bot is online
	if err := corradeClient.Tell(cfg.Prompts.WelcomeMessage); err != nil {
		log.Printf("Failed to announce online status: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down...")

	// Graceful shutdown
	cancel()
	time.Sleep(2 * time.Second)

	log.Println("Bot shutdown complete")
}
