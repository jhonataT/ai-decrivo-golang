# Decrivo

[![ci](https://github.com/jhonataT/ai-decrivo-golang/actions/workflows/ci.yml/badge.svg)](https://github.com/jhonataT/ai-decrivo-golang/actions/workflows/ci.yml)

App desktop para revisar branches com a ajuda de um agente de código, mas com a palavra final sempre de quem está revisando.

O agente (Claude Code, ou qualquer cliente MCP) lê o diff e deixa comentários ancorados nas linhas. O Decrivo mostra esses comentários ao lado do diff, e você aceita, rejeita ou reescreve cada um. No final sai um resumo em Markdown só com o que você aprovou, pronto para colar no PR. Se você liberar, o próprio agente publica esses comentários no PR do GitHub.

Comecei o projeto porque revisão automática que comenta direto no PR gera muito ruído. Eu queria a primeira passada da IA, mas com um filtro humano antes de qualquer coisa chegar ao time.

> Projeto em desenvolvimento. Uso no dia a dia e vou ajustando conforme aparecem as dores. Por enquanto só testei no Windows.

## Como funciona

```
┌──────────────┐   MCP (HTTP)    ┌───────────────────────────┐
│ Claude Code  │ ──────────────▶ │ Decrivo                   │
│ (ou outro    │  start_review   │  - lê o diff com git      │
│  cliente)    │  get_diff       │  - guarda achados em JSON │
│              │  add_finding    │  - UI para os vereditos   │
│              │  finish_review  │                           │
│              │  get_publish... │                           │
└──────┬───────┘                 └───────────────────────────┘
       │ gh api (só se você liberar)
       ▼
┌──────────────┐
│  PR GitHub   │
└──────────────┘
```

1. Você pede ao agente para revisar uma branch.
2. O agente chama `start_review` com o caminho do repositório, a branch base e a branch a revisar. O Decrivo roda `git merge-base` e `git diff` e devolve a lista de arquivos alterados.
3. Para cada arquivo, o agente pega o diff numerado (`get_diff`), registra os achados (`add_finding`) e opcionalmente explica o papel do arquivo no projeto (`set_file_context`).
4. Com `finish_review`, o agente manda um resumo e uma recomendação inicial (approve, request_changes ou comment). A janela do Decrivo vem para frente.
5. Você revisa achado por achado, ajusta o texto se precisar e finaliza escolhendo a decisão geral (aprovar, apenas comentar ou solicitar alterações). O Markdown gerado só inclui o que foi aceito.
6. Opcional: se você marcar **publicar no PR** ao finalizar, peça ao agente para publicar. Ele chama `get_publishable_review`, que devolve o payload da review do GitHub só com os achados aceitos (já com os seus ajustes) e com a sua decisão, e publica com o `gh`. Sem essa marcação nada é publicado.

### Publicação no PR

- Os comentários de linha ficam ancorados no commit que foi revisado (`commit_id`). Se a branch no GitHub tiver avançado depois da revisão, o agente para e avisa em vez de comentar em linhas que não batem mais.
- Achados sobre o arquivo inteiro (linha 0) vão para o corpo da review, junto com o resumo das alterações.
- Tudo sai numa única review, e o Decrivo registra a publicação (`mark_published`) para não publicar a mesma revisão duas vezes.
- O GitHub não permite aprovar nem solicitar alterações no próprio PR. Nesse caso, finalize com "Apenas comentar".

Lockfiles, imagens, fontes e arquivos deletados ficam fora do diff. Arquivos com patch muito grande (acima de ~60 KB) também são ignorados.

## Ferramentas MCP

| ferramenta | o que faz |
|---|---|
| `start_review` | abre uma revisão e devolve o `reviewId` e os arquivos alterados |
| `list_changed_files` | lista os arquivos de uma revisão |
| `get_diff` | diff de um arquivo, com o número da linha no arquivo novo à esquerda |
| `add_finding` | registra um achado (`praise` ou `improvement`, severidade `info`, `minor` ou `major`) |
| `set_file_context` | descreve em uma ou duas frases a responsabilidade do arquivo |
| `finish_review` | encerra a análise e traz a janela para frente |
| `get_publishable_review` | depois do seu veredito, devolve o payload da review do GitHub com os achados aceitos |
| `mark_published` | registra que a revisão foi publicada no PR |

O servidor escuta só em `127.0.0.1:7337/mcp`, sem autenticação. Não exponha essa porta.

## Rodando localmente

Pré-requisitos:

- Go 1.25+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- Git no `PATH`
- [GitHub CLI (`gh`)](https://cli.github.com/) no `PATH` e autenticado (`gh auth login`), para publicar no PR. O agente precisa ter acesso ao `gh`: no Claude Code, autorize `gh pr view` e `gh api` quando ele pedir, ou adicione esses comandos à lista de permissões. Sem o `gh` o resto funciona normalmente, e você cola o Markdown no PR à mão
- WebView2 no Windows (já vem no Windows 10/11 atualizado)

```sh
git clone https://github.com/jhonataT/ai-decrivo-golang.git
cd ai-decrivo-golang
go tool templ generate
wails dev
```

Para gerar o executável:

```sh
go tool templ generate
wails build
```

O binário sai em `build/bin/`.

Os arquivos `*_templ.go` não são versionados. Sempre que mexer em `internal/ui/pages.templ`, rode `go tool templ generate` de novo.

### Conectando o Claude Code

Com o Decrivo aberto:

```sh
claude mcp add --transport http decrivo http://127.0.0.1:7337/mcp
```

Depois é só pedir algo como *"revisa a branch feat/x contra a main no repositório G:\projetos\api usando o decrivo"*.

Para publicar no PR, confirme antes que o `gh` enxerga o repositório: rode `gh auth status` e `gh pr view feat/x` dentro dele. Depois de finalizar no Decrivo com "publicar no PR" marcado, rode `/decrivo-publish rev-3` (ou peça *"publica a revisão rev-3 no PR"*). O comando confere o repositório, o login do `gh` e se o PR ainda está no commit revisado antes de publicar.

## Onde ficam os dados

Cada revisão vira um arquivo JSON na pasta de configuração do usuário:

- Windows: `%AppData%\Decrivo\reviews`
- macOS: `~/Library/Application Support/Decrivo/reviews`
- Linux: `~/.config/Decrivo/reviews`

O arquivo tem um campo de versão do schema, para dar para migrar o formato sem perder histórico. Se um arquivo estiver corrompido, o app abre mesmo assim e só registra o erro no log.

## Estrutura

```
main.go               sobe o servidor MCP e a janela Wails
internal/
  gitrepo/            wrapper do git (merge-base, diff, filtro de arquivos)
  review/             regras da revisão: achados, vereditos, resumo final
  jsonstore/          persistência em JSON, um arquivo por revisão
  mcpserver/          ferramentas MCP expostas ao agente
  ui/                 páginas em templ, servidas para o HTMX
frontend/dist/        HTML, CSS e JS estáticos (HTMX, sem bundler)
```

A interface é renderizada no servidor com [templ](https://templ.guide) e atualizada com [HTMX](https://htmx.org). Não tem build de frontend, o Wails só serve os arquivos de `frontend/dist` e repassa as rotas `/ui/*` para o handler em Go.

## Testes

```sh
go tool templ generate
go test ./...
```

## Próximos passos

- [ ] Testar e empacotar para macOS e Linux
- [x] Publicar o comentário direto no PR (GitHub) depois do veredito
- [ ] Porta e caminho de dados configuráveis
- [ ] Release com binário para Windows

## Licença

[MIT](LICENSE)
