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

func main() {
	ctx, ctxCancel := context.WithCancel(context.Background())

	dvc := keylogger.FindKeyboardDevice()
	if dvc == "" {
		log.Println("no keyboard found. exit")
		return
	}

	kl, err := keylogger.New(dvc)
	if err != nil {
		log.Panicf("failed to create keylogger: %v", err)
	}
	eventChan := kl.Read()

	wg := sync.WaitGroup{}
	heatMap := heatmap{
		l: &sync.RWMutex{},
		m: make(map[string]int, 101),
	}
	ticker := time.NewTicker(time.Hour)
	wg.Add(1)
	logKey(ctx, eventChan, &wg, &heatMap, ticker)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)

	<-sigChan
	ctxCancel()
	wg.Wait()
	log.Println("main exit")
}

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

func logKey(ctx context.Context, eventChan <-chan keylogger.InputEvent, wg *sync.WaitGroup, heatMap *heatmap, ticker *time.Ticker) {
	defer wg.Done()

	go func() {
		for {
			select {
			case event := <-eventChan:
				if event.Type == keylogger.EvKey && event.KeyPress() {
					heatMap.Incr(event.KeyString())
				}
			case <-ticker.C:
				b := heatMap.Serialize()
				err := ioutil.WriteFile("heatmap.json", b, os.ModePerm)
				if err != nil {
					log.Printf("failed to write data to file: %v", err)
				}
				log.Println(string(b))
			case <-ctx.Done():
				ticker.Stop()
				log.Println("logKey exit")
				return
			}
		}
	}()
}
