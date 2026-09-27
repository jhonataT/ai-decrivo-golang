# Decrivo

[![ci](https://github.com/jhonataT/ai-decrivo-golang/actions/workflows/ci.yml/badge.svg)](https://github.com/jhonataT/ai-decrivo-golang/actions/workflows/ci.yml)

App desktop para revisar branches com a ajuda de um agente de código, mas com a palavra final sempre de quem está revisando.

O agente (Claude Code, ou qualquer cliente MCP) lê o diff e deixa comentários ancorados nas linhas. O Decrivo mostra esses comentários ao lado do diff, e você aceita, rejeita ou reescreve cada um. No final sai um resumo em Markdown só com o que você aprovou, pronto para colar no PR.

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
└──────────────┘                 └───────────────────────────┘
```

1. Você pede ao agente para revisar uma branch.
2. O agente chama `start_review` com o caminho do repositório, a branch base e a branch a revisar. O Decrivo roda `git merge-base` e `git diff` e devolve a lista de arquivos alterados.
3. Para cada arquivo, o agente pega o diff numerado (`get_diff`), registra os achados (`add_finding`) e opcionalmente explica o papel do arquivo no projeto (`set_file_context`).
4. Com `finish_review`, o agente manda um resumo e uma recomendação inicial (approve, request_changes ou comment). A janela do Decrivo vem para frente.
5. Você revisa achado por achado, ajusta o texto se precisar e finaliza. O Markdown gerado só inclui o que foi aceito.

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

O servidor escuta só em `127.0.0.1:7337/mcp`, sem autenticação. Não exponha essa porta.

## Rodando localmente

Pré-requisitos:

- Go 1.25+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- Git no `PATH`
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
- [ ] Publicar o comentário direto no PR (GitHub) depois do veredito
- [ ] Porta e caminho de dados configuráveis
- [ ] Release com binário para Windows

## Licença

[MIT](LICENSE)
