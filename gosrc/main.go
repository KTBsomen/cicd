package main

import (
	"fmt"
	"gosrc/parser"
	"gosrc/webhook"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var config parser.Config
	config.Parse()
	go webhook.StartWebhook(&config)

	fmt.Println(config.AdminEmail)
	fmt.Println(config.PublicIP)
	fmt.Println(config.Webhook)
	fmt.Println(config.RepoURL)
	fmt.Println(config.ServiceDir)
	stop := make(chan os.Signal, 1)
	// systemd sends SIGTERM on stop, SIGINT on Ctrl+C
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	fmt.Println("🚀 Services running under systemd...")

	// Block main
	<-stop
	fmt.Println("🛑 Stopped by system signal")
}
