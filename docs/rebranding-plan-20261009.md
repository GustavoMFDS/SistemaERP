# Preparação da nova marca do PDV

Issue #26. Esta mudança é **preparatória**: o proprietário ainda não escolheu o nome final.

## Troca segura

- O nome público é lido de `VITE_APP_BRAND_NAME` em build-time, com fallback SistemaEmGo até aprovação do nome final.
- Pontos já centralizados: cabeçalho do aplicativo, título da aba, ativação de funcionário e cabeçalho de comprovante não fiscal.
- Exemplo para validar uma opção: `VITE_APP_BRAND_NAME="Lojanto" npm run build` (em PowerShell: `$env:VITE_APP_BRAND_NAME='Lojanto'; npm run build`).
- Nome final deverá também atualizar metadados estáticos em `web/index.html`, PWA `web/public/manifest.webmanifest`, ícones e nome do instalável, textos de e-mail, documentação de produto e material de suporte. Não alterar os identificadores de PWA, banco, rotas ou migrações em uma mera mudança visual.
- Preservar namespaces históricos como `sistemaemgo:productsCache:v2`, fila offline, caixa, refresh tokens e carrinhos suspensos: uma renomeação indiscriminada pode deixar pedidos pendentes ocultos e bloquear reconciliação.
- Go module `github.com/example/sistemaemgo` e o nome do repositório ficam como nomes **técnicos legados** até existir decisão separada e plano de migração de imports, CI, imagens Docker e backups.
- A nova marca não altera titularidade/CNPJ, dados fiscais, emissão e provas legais.

## Candidatos ainda não aprovados

- **Lojanto** — simples de falar, associa a loja e gestão.
- **Balcavo** — remete a balcão, opção mais próxima do PDV físico.
- **Varelum** — mais abstrato, pode crescer de PDV para ERP.

São ideias para discussão, **não possuem autorização de uso comercial**. A consulta web inicial não substitui busca de marcas no INPI, disponibilidade de domínio e análise jurídica. Não atualizar ícones, manifest definitivo ou material público sem confirmar uma escolha.

## Gates

1. Proprietário escolhe nome.
2. Verificar INPI, domínio e canais.
3. Executar atualização total da marca em UI/PWA/arquivos públicos.
4. Validar instalação/upgrade do PWA sem perder caches/filas.
5. Confirmar com testes E2E, mantendo o PR #15 e os gates fiscais independentes.
