package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/MarinX/keylogger"
)

type heatmap struct {
	m map[string]int
	l *sync.RWMutex
}

func (hm *heatmap) Incr(key string) {
	hm.l.Lock()
	hm.m[key]++
	hm.l.Unlock()
}

func (hm *heatmap) Serialize() []byte {
	hm.l.RLock()
	b, err := json.Marshal(hm.m)
	if err != nil {
		return []byte(fmt.Sprintf(`{"error": "%s"}`, err))
	}
	hm.l.RUnlock()
	return b
}

func main() {
	f, err := os.OpenFile("gatekeeper.log", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		log.Panicf("failed to open log file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// Keylogger
	dvc := keylogger.FindKeyboardDevice()
	if dvc == "" {
		log.Println("no keyboard found. exit")
		return
	}
	kl, err := keylogger.New(dvc)
	if err != nil {
		log.Panicf("failed to create keylogger: %v", err)
	}
	defer kl.Close()
	eventChan := kl.Read()

	// Start background worker to log keys
	ctx, ctxCancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	heatMap := heatmap{
		l: &sync.RWMutex{},
		m: make(map[string]int, 101),
	}
	ticker := time.NewTicker(time.Minute*5)
	defer ticker.Stop()
	wg.Add(1)
	go logKey(ctx, eventChan, &wg, &heatMap, ticker)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)
	<-sigChan
	ctxCancel()
	wg.Wait()
	log.Println("main exit")
}

func logKey(ctx context.Context, eventChan <-chan keylogger.InputEvent, wg *sync.WaitGroup, heatMap *heatmap, ticker *time.Ticker) {
	defer wg.Done()

	for {
		select {
		case event := <-eventChan:
			if event.Type == keylogger.EvKey && event.KeyPress() {
				heatMap.Incr(event.KeyString())
			}
		case <-ticker.C:
			b := heatMap.Serialize()
			err := ioutil.WriteFile("heatmap.json", b, 0666)
			if err != nil {
				log.Printf("failed to write data to file: %v", err)
			}
			log.Println(string(b))
		case <-ctx.Done():
			log.Println("logKey exit")
			return
		}
	}
}
