package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
)

func main() {
	mechanism, _ := scram.Mechanism(scram.SHA512, "traffic-broker", os.Getenv("KAFKA_PW"))
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, SASLMechanism: mechanism, TLS: &tls.Config{InsecureSkipVerify: true}}
	part, _ := strconv.Atoi(os.Args[1])
	off, _ := strconv.ParseInt(os.Args[2], 10, 64)
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: []string{os.Args[3]}, Topic: "analysis.receipts.v1", Partition: part, Dialer: dialer, MinBytes: 1, MaxBytes: 10 << 20, MaxWait: 300 * time.Millisecond})
	defer r.Close()
	_ = r.SetOffset(off)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for i := 0; i < 6; i++ {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			fmt.Println("fetch:", err)
			return
		}
		fmt.Printf("off=%d %s\n", m.Offset, string(m.Value[:min(220, len(m.Value))]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
