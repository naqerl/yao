package main

import (
        "bufio"
        "context"
        "fmt"
        "log"
        "log/slog"
        "os"

        "github.com/firebase/genkit/go/ai"
        "github.com/firebase/genkit/go/genkit"
        "github.com/firebase/genkit/go/plugins/googlegenai"

        "github.com/naqerl/yao/tool"
)

func main() {
        ctx := context.Background()
        slog.Info("starting")

        g := genkit.Init(ctx,
                genkit.WithDefaultModel("googleai/gemini-2.5-flash"),
                genkit.WithPlugins(&googlegenai.GoogleAI{}),
        )

        bashTool := tool.DefineBash(g)

        var chat []*ai.Message
        agent := genkit.DefineFlow(g, "agent", func(ctx context.Context, task string) (string, error) {

                chat = append(chat, ai.NewUserMessage(ai.NewTextPart(task)))
                resp, err := genkit.Generate(ctx, g,
                        ai.WithTools(bashTool),
                        ai.WithMessages(chat...),
                )
                chat = resp.History()

                return resp.Text(), err
        })

        slog.Info("genkit inited")

        reader := bufio.NewReader(os.Stdin)

        for {
                select {
                case <-ctx.Done():
                        os.Exit(0)
                default:
                        fmt.Print("> ")
                        problem, err := reader.ReadString('\n')
                        if err != nil {
                                log.Fatalf("could not read from stdin")
                        }

                        resp, err := agent.Run(ctx, problem)
                        if err != nil {
                                slog.Error("failed to run flow", "with", err)
                        }
                        fmt.Println(resp)

                }
        }
}
