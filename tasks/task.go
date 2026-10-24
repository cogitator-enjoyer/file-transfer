package tasks

import (
	"file-transfer/client"
	"fmt"
	"time"
)

var taskMap map[client.EventType]func() (client.ResponseDiscovery, error) = map[client.EventType]func() (client.ResponseDiscovery, error){
	client.Discover: discovery,
}

func IsTask(eventType client.EventType) bool {
	_, ok := taskMap[eventType]
	return ok
}

func Launch(message client.Message, responseFn func(eventType client.EventType, data any)) error {

	fn, ok := taskMap[message.Type]
	if !ok {
		return fmt.Errorf("%v eventType is not registered in tasks", message.Type)
	}
	go func() {
		data, err := fn()
		if err != nil {
			fmt.Printf("%v task failed with error %e", message.Type, err)
		}
		responseFn(message.Type, data)
	}()
	return nil
}

func discovery() (client.ResponseDiscovery, error) {
	fmt.Println("launch discovery task")
	time.Sleep(time.Second * 2)
	fmt.Println("discovery task done")
	return client.ResponseDiscovery{Discovered: []string{"192.165.1.5"}}, nil
}
