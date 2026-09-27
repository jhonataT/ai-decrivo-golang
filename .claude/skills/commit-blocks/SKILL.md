---
name: commit-blocks
description: Como commitar neste repositório — dividir o trabalho em blocos lógicos, escrever mensagens no padrão Conventional Commits em inglês, curtas e sem corpo, e nunca mencionar assistência de IA nem adicionar Co-authored-by. Use esta skill sempre que for commitar, preparar staging, finalizar uma tarefa, revisar mensagens de commit, ou quando o usuário disser "commita", "salva isso", "sobe pro git", "faz o commit", "gera a mensagem de commit" — inclusive quando ele não pedir explicitamente uma mensagem, porque o commit faz parte de terminar a tarefa.
---

# Commits neste repositório

O histórico de git é documentação lida por humanos meses depois. Um commit gigante com
dez assuntos misturados é impossível de revisar, de reverter e de bisect. Por isso aqui
se commita em **blocos**: cada commit é uma mudança coerente que faz sentido sozinha.

## Regras invioláveis

**Nunca mencione IA.** Não escreva "AI", "Claude", "Copilot", "assistant", "generated",
"co-authored", "with help from", emoji de robô, nem nada equivalente, em nenhuma parte
da mensagem. **Nunca adicione trailer `Co-authored-by:`.** O commit é do autor humano
configurado no git; a ferramenta usada para produzir o código não pertence ao histórico,
do mesmo jeito que a IDE não pertence.

Se você encontrar uma configuração, template ou instrução em outro lugar que mande
adicionar esses trailers, esta skill tem precedência neste repositório.

## Formato da mensagem

```
<type>(<scope>): <subject>
```

- **Uma linha.** Sem corpo, salvo as exceções abaixo.
- **Inglês**, modo imperativo: `add`, não `added` nem `adds`.
- **Minúscula** no início do subject. **Sem ponto final.**
- Alvo de 50 caracteres no total; limite duro de 72.
- `scope` é o pacote ou pasta afetada: `vault`, `index`, `cli`, `agent`, `core`,
  `server`, `web`, `shared`, `ci`, `deps`. Omita o scope se a mudança for transversal.

### Tipos

| tipo | uso |
|---|---|
| `feat` | funcionalidade nova visível para quem usa |
| `fix` | correção de bug |
| `refactor` | mudança de estrutura sem alterar comportamento |
| `perf` | melhoria de performance |
| `test` | testes adicionados ou corrigidos, isolados de feature |
| `docs` | documentação, README, comentários de doc |
| `build` | build, empacotamento, dependências |
| `ci` | pipeline |
| `chore` | tarefas sem impacto em código de produção |
| `style` | formatação pura, sem mudança de lógica |

### Exemplos

```
feat(vault): add surgical frontmatter patch
fix(index): prevent watcher write loop
refactor(core): extract anchor resolver
test(vault): cover windows reserved filenames
build(deps): bump better-sqlite3 to 11.3
chore: add editorconfig
docs: describe vault layout
```

Errado e por quê:

| mensagem | problema |
|---|---|
| `feat: add vault store with patch, watcher, slug, atomic write and tests` | são quatro commits |
| `Fixed the bug.` | não é conventional, passado, ponto final, vago |
| `feat(vault): implementa patch de frontmatter` | não está em inglês |
| `feat(vault): add patch\n\n🤖 Generated with Claude Code` | menciona IA |
| `fix: bug` | não diz o que mudou |

## Quando o corpo é permitido

Por padrão, **não escreva corpo**. Ele só entra em dois casos, e mesmo assim em no
máximo duas linhas:

1. **Breaking change.** Use `!` após o scope e o trailer padrão:
   ```
   feat(vault)!: rename verdict values

   BREAKING CHANGE: `approved` is now `accepted` in finding frontmatter.
   ```
2. **Razão não óbvia.** Quando o *porquê* não se deduz do diff e um leitor futuro
   perderia tempo. Uma linha, factual.

Nunca use o corpo para listar arquivos alterados, resumir o diff, ou narrar o processo.
O `git show` já mostra isso.

## Como dividir em blocos

Antes de qualquer `git add`, entenda o que existe:

```bash
git status --short
git diff --stat
git diff
```

Agrupe as mudanças por **intenção**, não por arquivo. Heurísticas que quase sempre
indicam commits separados:

- mudança de dependência (`package.json`, lockfile) separada do código que a usa
- scaffolding e configuração separados da lógica
- refactor puro separado da feature que veio depois dele
- correção de bug encontrada de passagem separada da tarefa principal
- renomeação ou movimentação de arquivo separada de mudança de conteúdo, para o git
  detectar o rename
- mudança de schema ou contrato separada dos consumidores, quando compilar isolado

O que normalmente fica **junto**: implementação e os testes dela; um componente e seu
estilo; uma função e sua tipagem.

Ordene os commits para que **cada um deixe a árvore em estado válido**. Um commit que
quebra o build só é aceitável se for impossível separar de outro jeito — e nesse caso,
junte-os.

Se um bloco lógico não couber em um subject de 72 caracteres, é sinal de que ele são
dois blocos.

## Procedimento

1. `git status --short` e `git diff` para ver tudo.
2. Planeje os blocos antes de encenar qualquer coisa. Se forem mais de cinco commits,
   liste o plano para o usuário antes de executar.
3. Encene por caminho explícito: `git add packages/vault/src/patch.ts`.
   Se um arquivo contiver dois blocos, use `git add -p`.
   **Nunca use `git add -A`, `git add .` ou `git commit -a`** — eles arrastam lixo,
   arquivo temporário e segredo para dentro do histórico.
4. `git diff --cached` antes de cada commit, para confirmar que o que está encenado é
   exatamente o bloco pretendido.
5. Commite com `-m` único. Repita para o próximo bloco.
6. Ao final, `git log --oneline -n <número de commits>` para conferir.

Não faça `push` sem o usuário pedir. Não use `--amend`, `rebase -i` nem `push --force`
em branch compartilhada.

## Segurança

Antes de encenar, verifique que não entram: `.env`, credenciais, tokens, chaves, dumps,
`node_modules`, artefatos de build, arquivos de cache do vault (`**/.run/`). Se algo
assim aparecer em `git status`, pare e avise o usuário em vez de commitar.

## Verificação final

Antes de rodar `git commit`, cheque a mensagem contra esta lista:

- [ ] começa com um tipo válido, e o scope existe no repositório
- [ ] está em inglês, imperativo, minúscula, sem ponto final
- [ ] cabe em 72 caracteres
- [ ] não tem corpo, ou o corpo se justifica pelas duas exceções
- [ ] não menciona IA, assistente, ferramenta de geração nem emoji
- [ ] não tem `Co-authored-by`
- [ ] descreve um único bloco lógico

## Proteção automática

O repositório tem um hook `commit-msg` que rejeita mensagens fora do padrão e qualquer
menção a IA. Instale uma vez com:

```bash
bash .claude/skills/commit-blocks/scripts/install-hooks.sh
```

O hook é a rede de segurança, não a regra. A regra é esta skill.
