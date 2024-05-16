// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a TURN client using UDP
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v3"
)

func main() {
	turnserver := "turn.cloudflare.com:3478"
	user := "{add username}"
	credential := "{add credential}"
	realm := "turn.cloudflare.com"

	flag.Parse()

	udpConn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		log.Panicf("Failed to listen: %s", err)
	}

	defer func() {
		if closeErr := udpConn.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	tcpConn, err := net.Dial("tcp", turnserver)
	if err != nil {
		log.Panicf("Failed to listen: %s", err)
	}

	defer func() {
		if closeErr := tcpConn.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	udpClientConfig := &turn.ClientConfig{
		STUNServerAddr: turnserver,
		TURNServerAddr: turnserver,
		Conn:           udpConn,
		Username:       user,
		Password:       credential,
		Realm:          realm,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	}

	tcpClientConfig := &turn.ClientConfig{
		STUNServerAddr: turnserver,
		TURNServerAddr: turnserver,
		Conn:           turn.NewSTUNConn(tcpConn),
		Username:       user,
		Password:       credential,
		Realm:          realm,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	}

	udpClient, err := turn.NewClient(udpClientConfig)
	if err != nil {
		log.Panicf("Failed to create TURN client: %s", err)
	}
	defer udpClient.Close()

	tcpClient, err := turn.NewClient(tcpClientConfig)
	if err != nil {
		log.Panicf("Failed to create TURN client: %s", err)
	}
	defer tcpClient.Close()

	err = udpClient.Listen()
	if err != nil {
		log.Panicf("Failed to listen udp: %s", err)
	}

	err = tcpClient.Listen()
	if err != nil {
		log.Panicf("Failed to listen tcp: %s", err)
	}

	relayConnUDP, err := udpClient.Allocate()
	if err != nil {
		log.Panicf("Failed to allocate: %s", err)
	}

	defer func() {
		if closeErr := relayConnUDP.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	relayConnTCP, err := tcpClient.Allocate()
	if err != nil {
		log.Panicf("Failed to allocate: %s", err)
	}

	defer func() {
		if closeErr := relayConnUDP.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	log.Printf("relayed-address-udp=%s", relayConnUDP.LocalAddr().String())
	log.Printf("relayed-address-tcp=%s", relayConnTCP.LocalAddr().String())

	// send computer -> relayConnTCP -> relayConnUDP -> computer

	dest := relayConnUDP.LocalAddr()
	src := relayConnTCP
	udpClient.CreatePermission(src.LocalAddr())
	go listenLoop(relayConnTCP)
	go listenLoop(relayConnUDP)
	sendLoop(src, dest)
}

func sendLoop(src net.PacketConn, dst net.Addr) {
	var bytesSent int64 = 0
	ticker := time.NewTicker(1 * time.Second)
	startTime := time.Now()
	defer ticker.Stop()
	randomBytes := make([]byte, 1200) // random amount of bytes still under general internet MTU

	for {
		select {
		case <-ticker.C:
			duration := time.Since(startTime)
			startTime = time.Now()
			mbps := (float64(bytesSent) * 8) / (float64(duration.Seconds()) * 1e6)
			log.Printf("Current send speed: %.2f Mbps", mbps)
			bytesSent = int64(0)
		default:
		}
		_, err := rand.Read(randomBytes)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		n, err := src.WriteTo(randomBytes, dst)
		if err != nil {
			log.Println(err)
		}
		bytesSent = bytesSent + int64(n)
	}

}

func listenLoop(c net.PacketConn) {
	buf := make([]byte, 1600)
	var bytesreceived int64 = 0
	ticker := time.NewTicker(1 * time.Second)
	startTime := time.Now()
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			duration := time.Since(startTime)
			startTime = time.Now()
			mbps := (float64(bytesreceived) * 8) / (float64(duration.Seconds()) * 1e6)
			log.Printf("Current receive speed: %.2f Mbps", mbps)
			bytesreceived = int64(0)
		default:
		}
		n, _, readerErr := c.ReadFrom(buf)
		if readerErr != nil {
			log.Println(readerErr)
		}
		bytesreceived = bytesreceived + int64(n)
	}
}
