package main

import (
  "context"
  "fmt"
  "os"

  "mini-atoms/internal/generation"
)

func main() {
  c := generation.NewDeepSeekClient(generation.DeepSeekClientConfig{
    APIKey: os.Getenv("DEEPSEEK_API_KEY"),
    AppBaseURL: "http://localhost:8080",
  })
  out, err := c.GenerateSpecJSON(context.Background(), generation.ClientRequest{
    UserPrompt: "请生成一个最小可用的待办应用，包含 form、list、toggle、stats。",
  })
  if err != nil {
    panic(err)
  }
  fmt.Println(out)
}
