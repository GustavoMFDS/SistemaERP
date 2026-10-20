# Frontend do SistemaERP (interface ainda identificada como SistemaEmGo)

Aplicação React 19 + TypeScript + Vite + Tailwind, com PDV, catálogo, estoque, compras, clientes, financeiro, fiscal e suporte de instalação como PWA.

## Requisitos e execução

- Node.js 22.x e npm.
- API Go e PostgreSQL/Redis ativos, com seed **apenas em desenvolvimento** quando necessário.

Na raiz do repositório, use primeiro as instruções do [README principal](../README.md). Depois:

```bash
cd web
npm ci
npm run dev
```

Interface: http://localhost:5173. API local padrão: http://localhost:8080. Use `VITE_API_BASE_URL` no ambiente do Vite ou em `web/.env.local` para sobrescrever a URL; **o `.env` da raiz é usado pelo Docker Compose, não carregado automaticamente pelo Vite neste subdiretório**.

## Qualidade

```bash
npm run lint
npm run build
```

`npm run test:e2e` usa Playwright e precisa da infraestrutura apropriada (consulte `playwright.config.ts` e `../docs/smoke-test.md`). Alguns cenários usam API simulada e não substituem a integração real.

## PDV offline e impressão

A fila local preserva intenções de venda antes do envio e tenta sincronização quando a conexão volta. Não apague dados do navegador nem desinstale a PWA antes de reconciliar pendências. Venda offline pendente **não** representa nota fiscal emitida.

A impressão do DANFE usa diálogo do navegador e só deve ficar disponível para documento fiscal efetivamente autorizado; não há impressão silenciosa por ESC/POS nativa. Detalhes em [PDV offline](../docs/offline-pdv.md) e [impressão fiscal semi-automática](../docs/pdv-impressao-fiscal-semi-automatica.md).

A identidade visual ainda contém `SistemaEmGo` em chaves e textos por compatibilidade. A mudança definitiva deve ser feita em PR próprio.
