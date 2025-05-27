// internal/agent/user_proxy.go
package agent

import (
    "bufio"
    "fmt"
    "os"
    "aiupstart.com/go-gen/internal/model"
)

type UserProxyAgent struct {
    name string
}

func NewUserProxyAgent(name string) *UserProxyAgent {
    return &UserProxyAgent{name: name}
}

func (u *UserProxyAgent) Name() string { return u.name }

func (u *UserProxyAgent) Start(input <-chan model.Message, output chan<- model.Message) {
    go func() {
        // Listen for messages destined for user and print to console
        for msg := range input {
            fmt.Printf("[From %s]: %s\n", msg.Sender, msg.Content)
        }
    }()
}

func (u *UserProxyAgent) UserInputLoop(output chan<- model.Message) {
    scanner := bufio.NewScanner(os.Stdin)
    for {
        fmt.Print("[You]: ")
        if scanner.Scan() {
            userInput := scanner.Text()
            output <- model.Message{Sender: u.name, Content: userInput}
        }
    }
}