# Baseline e auditoria inicial

**Data:** 24/11/2026. Inspeção estática do GitHub, não execução do sistema. Main: 69d5e205400d3f4b02c35dca9b164bdeaa9cf49c, último commit em 24/10/2026. Árvore: b018d07bcb007afc8400d0d4063fb7f5419b816d. 52 migrações UP, 17 arquivos *Page.tsx. Contagens de artefatos não são contagens de fluxos aprovados.

## Inventário
| Área | Evidência no código/documentação | Próxima verificação |
|---|---|---|
| Vendas/caixa | sales, PDVPage, idempotência, sangria/suprimento | corrida entre caixas, troco, cancelamento, falha de rede |
| Catálogo/estoque | inventory, fotos, variantes, importações, saldos e histórico | contagem formal, custo histórico, exportação e concorrência |
| Compras/devoluções | procurement, returns, recebimento parcial e reembolso | efeitos cruzados e repetição segura |
| Financeiro | finance, ledger, contas, conciliação, ranking | definição de margem, liquidação e divergências |
| Clientes/equipe | customers, team, auth, permissões e recuperação | vínculo cliente-venda, isolamento, acesso por perfil |
| Fiscal | fiscal, SEFAZ provider, fila e arquivo | homologação real por emitente, aprovação contábil; NF-e 55 não pronta |
| Infra | scripts Windows, PostgreSQL, Redis/Garnet, backup, HTTPS | instalação assinada, reinício, restauração física e periféricos |
| Frontend | React, PWA, Playwright, Home, Reports, Setup | fluxos reais por persona, acessibilidade, offline e dispositivos |
| Segurança | auth/RBAC, privacidade, auditoria, rate limit | revisão por rota/repositório, abuso, dependências, segredos |
| Qualidade | CI, testes Go/integração/E2E e runbooks | repetir em SHA exato, separar simulação de teste real |

## PRs abertas no snapshot (24)
- #216 consolida #212–#215 (draft); #211 verificação de assinatura em planejamento (draft). Revisar cobertura de #211 na #216, não presumir.
- #212 instalador; #213 correções/validação local; #214 reruns fiscais; #215 Tailwind 4.
- Históricas empilhadas: #136, #135, #123, #122, #75, #74, #73, #66, #64, #62, #57, #56, #55, #53, #31, #30, #27. #8 antiga com base main.
- Todas exceto #8 estavam em draft. Não fechar por título; comparar arquivos, testes, contratos, comportamento e base; classificar integrado/substituído, ainda necessário, conflito ou bloqueado, com evidência e revisão.
- #216 HEAD observado: 369b7bc0646a7755419f0288960732511b125189. Descrição registra CI 20/20 no commit ac83fe5b7e21ffaf1043f25a7a6a66de2c8c3a50, run [41907778163](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907778163); não extrapolar resultado ao HEAD observado. Setup [41907777994](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907777994) com gates aplicáveis e jobs ignorados. Não reexecutado nesta auditoria.
- Issues relevantes: #151 (checklist), #148/#194 (dependências), #160 (isolamento E2E), #29 (rerun), #32/#33 (fiscal). Reconciliar com código e PRs, não fechar automaticamente.

## Divergências e bloqueios
- README declara ausência de licença e 51 migrações, mas LICENSE contém MIT e árvore atual tem 52 UP. Corrigir documentação em tarefa própria, após conferir referência.
- Catálogos de 09/10 e 16/10 descrevem branches antigas; não tratar limitações antigas como confirmação de ausência no HEAD.
- Instalação gráfica, PWA/Zod, Tailwind 4 e correções recentes estão propostas na #216; não considerar integradas.
- Aceite físico ainda pendente: Windows limpo, reinício, LAN dois equipamentos, restauração externa, periféricos. A descrição da #216 registra VirtualBox E_ACCESSDENIED no ensaio anterior; estado atual da máquina não verificado.
- Fiscal real, credenciamento, regras de tributação, documentos genuínos e aceite contábil não comprovados. Pix/TEF não comprovados.
- Repositório público: revisar histórico e artefatos para exposição de informações, nunca publicar segredos ou dados reais.

## AUD-01 — plano de auditoria completa
1. Congelar SHA e registrar árvore, módulos, rotas, páginas, migrations, jobs, dependências; mapear cada capacidade a rota, domínio, persistência, UI e teste.
2. Revisar regras: dinheiro inteiro/decimal exato, transações, locking, idempotência, estados, tenant e RBAC; analisar SQL, índices, migrações e integridade referencial.
3. Executar em ambiente descartável: gofmt, go test, go vet, govulncheck, integração PG/Redis repetida, lint, build, auditoria npm, Playwright real e mock identificados separadamente.
4. Revisar UI por jornada e dispositivo; registrar screenshot sem PII, estados vazios/erro/offline, acessibilidade e tempo de tarefas.
5. Verificar deploy, backup e restore, jobs, logs, recuperação, exposição de rede, configuração e cadeia de suprimentos.
6. Mapear dependências externas e gaps fiscais/contábeis para validação por responsáveis.
7. Entregar matriz de achados com ID, severidade, evidência (arquivo/linha ou run/SHA), impacto, reprodução, recomendação, dono e aceite. Sem achados fabricados.

Severidade: P0 risco de dinheiro/dados/segurança/fiscal ou impedimento de piloto; P1 operação essencial; P2 melhoria; P3 oportunidade. Estado: não verificado, implementado, testado em ambiente X, validado externamente, bloqueado. Nunca usar percentual de prontidão sem denominador.
