# Fase 1 — Esterium Foundation (P0)

Objetivo: base integrada, testada e recuperável antes de piloto. Não autoriza uso comercial ou transmissão fiscal.

| ID | Entrega | Critério de aceite |
|---|---|---|
| FND-01 | Reconciliar PRs #216/#211 e cadeia histórica | Matriz por PR, diff e commits, cobertura funcional, checks HEAD, decisão justificada; sem merge/fechamento redundante |
| FND-02 | Documentação atualizada | Catálogo por SHA, licença MIT, migrações, capacidades e limites reconciliados |
| FND-03 | Instalador Windows e dependências | Setup integrado, origem/hash/assinatura de distribuição válidos, PostgreSQL/Garnet com TLS/ACL, instalação limpa sem Go/Node/Docker, reparo/reinício e falha segura |
| FND-04 | Backup e recuperação | Agendamento, alerta, destino externo protegido, duas gerações verificadas, restauração em outro computador de dados, imagens e artefatos pertinentes; RPO/RTO acordados e medidos |
| FND-05 | LAN e concorrência | Dois terminais físicos com HTTPS confiável, firewall privado, disputa da última unidade, replay idempotente, queda de rede e recuperação |
| FND-06 | Periféricos e piloto | Leitor, impressora, gaveta se usada, calibração 58/80 mm; rotina abertura/venda/devolução/fechamento; evidências e aceite humano |
| FND-07 | Homologação fiscal aplicável | Requisitos oficiais vigentes confirmados por emitente/UF, contador valida tributação, testes autorizados de emissão/consulta/cancelamento/contingência/inutilização, arquivo e DANFE genuínos; autorização explícita para produção |
| FND-08 | Segurança e performance | Revisão tenant/RBAC, scanners, dependências (incl. #148/#194), carga, falhas, logs, proteção de segredos e correções verificadas |
| FND-09 | Atualização e recuperação | Manifesto e assinatura verificados, backup recente, migração com preflight, falha simulada, recuperação testada e versão visível; automatizar só após prova |
| FND-10 | Piloto controlado | Loja e responsáveis definidos, treinamento, critérios de interrupção e fallback, aceite documentado; sem substituir processo fiscal válido antes de liberação |

## Sequência
AUD-01 e FND-01 primeiro. Depois código e CI da versão candidata, laboratório descartável, Windows limpo, testes físicos, validação externa. Gates físicos e fiscais não são substituídos por aprovação de CI. Piloto não fiscal somente em condições autorizadas e sem transações comerciais que exijam emissão fiscal não disponível.

## Evidência existente
Issue [#151](https://github.com/GustavoMFDS/SistemaERP/issues/151) registra testes descartáveis EDB, Garnet/NSSM, restauração em segundo cluster, imagens, concorrência HTTPS e manifesto. PR #216 registra Setup.exe e correções, mas continua draft. Usar runbooks [piloto local](../esterium-local-pilot-checklist.md), [instalação](../esterium-local-one-command-first-install.md), [backup](../esterium-local-backup-externo.md), [recuperação de atualização](../esterium-local-update-recovery.md) e [fiscal MG](../fiscal-mg-guia-operador-2026.md).

Bloqueadores: certificado/proveniência comercial, acesso a laboratório, equipamentos, credenciais/homologação fiscal e contador. Não tentar contornar privilégios, desativar proteções ou colocar certificados em Git.
