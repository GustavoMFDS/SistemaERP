# Esterium Local — aceite de instalação piloto (Windows x64)

**Uso:** uma folha de conferência para operador técnico e responsável da loja. **Não é autorização de operação comercial**; emissão fiscal real permanece bloqueada. Registrar evidências e assinar os testes; qualquer falha reprova o piloto.

| Gate | Prova observável | Resultado |
| --- | --- | --- |
| 1. Pacote confiável | Origem aprovada, versão registrada, arquivo intacto e `tools/verify-esterium-bundle.ps1` aprovado; pacote ainda **não possui assinatura oficial** | [ ] |
| 2. Equipamento e serviço | PC Windows x64 compatível, espaço e energia estável; serviço `EsteriumERP` sob `LocalService`, sem Go/Node/Docker necessários para **executar** a UI | [ ] |
| 3. Banco e segurança | PostgreSQL 16 e Redis compatível provisionados fora do bundle; inventário Windows em loopback, credenciais em arquivos com ACL restrita, TLS/certificados confiáveis, versão SQL correta e `--check-config` / `--check-dependencies` reais aprovados | [ ] |
| 4. Rede interna | Pelo menos **dois caixas físicos** acessam a mesma loja por HTTPS confiável, sem expor PostgreSQL/Redis na LAN; login e permissões segregados | [ ] |
| 5. PDV real | Produto/estoque, leitura de código, dinheiro e troco, cartão e Pix confirmados manualmente, fechamento, devolução, impressão de **teste não fiscal** e bobinas 58/80 mm | [ ] |
| 6. Falhas e recuperação | Venda pendente/offline não aparece como confirmada; queda de rede/reinício preserva dados e não duplica venda; caixa não fecha com pendências | [ ] |
| 7. Backup independente | Tarefa diária agendada, cópia obrigatória em outro computador, integridade/frescor conferidos e **restauração ensaiada em outra máquina** com dados sintéticos | [ ] |
| 8. Atualização segura | Backup restaurável antes de migração; nenhuma atualização automática/destrutiva; `esterium-migrate.exe` apenas empacotado e verificado, sem executá-lo sobre base real | [ ] |
| 9. Fiscal | Regra de bloqueio de transmissão ativa; não declarar NFC-e/NF-e autorizada. Aguardar credenciamento, protocolo SEFAZ autêntico, XML processado e aceite fiscal da UF/empresa | [ ] |

## Comandos de diagnóstico (executados pelo técnico, não pelo caixa)

Na pasta extraída de um pacote técnico **aprovado**, conferir os hashes:

```powershell
.\tools\verify-esterium-bundle.ps1 -BundleDir (Get-Location).Path
```

Com configurações seguras já provisionadas para a identidade de serviço, executar separadamente:

```powershell
.\esterium-server.exe --check-config
.\esterium-server.exe --check-dependencies
```

O segundo comando apenas testa conectividade, esquema e operações atômicas; **não migra o banco**. Para backup externo, usar `tools/check-esterium-backup-integrity.ps1` com os dois diretórios reais; ver `docs/esterium-local-backup-externo.md`. Não colocar senhas em comandos, ZIP, arquivos de log ou capturas.

## Decisão do piloto

- Local/data e versão do pacote: ______________________________
- Responsável técnico e evidência de restore: __________________
- Responsável pela loja, terminais e impressoras: ______________
- Pendências reprovadas e plano de correção: ____________________

**Critério:** todos os gates técnicos precisam de evidência real, não só CI. Ainda que os gates 1–8 passem, o gate 9 **proíbe liberação fiscal/uso comercial dependente de emissão** até homologação real e aprovação responsável. O desenvolvimento pode continuar em ambiente descartável com dados fictícios.
