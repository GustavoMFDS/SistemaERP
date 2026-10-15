# SistemaERP / SistemaEmGo — recuperação e integridade fiscal MG
**Revisão de 14/10/2026. Sem homologação oficial da SEF/MG.**

## Finalidade

O PDV deve conseguir finalizar uma venda e registrar sua **obrigação
fiscal** independentemente da escolha de impressão. A opção de
não imprimir **nunca** cancela a operação nem significa que a NFC-e
foi autorizada. Notas só são consideradas autorizadas quando o
protocolo e o XML processado (`nfeProc`) estão arquivados,
correspondem à chave correta e não tiveram o SHA-256 alterado.

Este endurecimento é conservador e vale para as automações de
**homologação em MG**. A transmissão automática de produção continua
desabilitada por padrão. O código não cria credenciamento SEFAZ,
CNPJ, certificado A1 ou homologação legal sem dados da empresa.

## Recuperação automática por estado local

Os documentos seguem as transições seguras:

| Documento local | Tratamento |
| --- | --- |
| Venda finalizada e não reservada | Planejamento automático somente quando empresa estiver configurada e opção de homologação explicitamente ligada |
| `reserved` / `signed` | Reavaliar configurações e prosseguir apenas conforme os guardas e tentativas do emissor |
| `submitted` | **Nunca retransmitir cegamente**; aguardar consulta da chave |
| Consulta `100` válida, protocolo e XML íntegros | Arquivar autorização sob transação e marcar filas concluídas |
| Resposta inconclusiva, timeout, 217 ou duplicidade | Preservar pendência e consultar/revisar; não gerar nova chave |
| Rejeição/cancelamento | Encaminhar a revisão; não criar outra nota automaticamente |
| Última tentativa interrompida (12 de emissão ou 10 de consulta) | Quando o lease expira, classificar como **manual_review** |
| `authorized` sem `nfeProc`, protocolo ou SHA-256 correspondente | **Alerta de integridade**, sem impressão nem download como NFC-e autorizada |

As operações de reconciliação `ReconcileNFCeAutomaticHomologationJobs`,
`ReconcileExhaustedNFCeConsultations` e
`EscalateTerminalNFCeConsultationReviews` apenas atualizam o
**controle local de filas**. Não fazem SOAP, não autorizam, não
corrigem um protocolo e não mudam o status da nota na tabela
`invoices`. São idempotentes; respeitam leases ainda válidos.
A rotina `ResolveNFCeConsultationPlans` só resolve consultas
autorizadas se o `nfeProc` possuir conteúdo e SHA íntegros.

## Como a equipe identifica e trata uma pendência

Em **Fiscal → Vendas e emissão fiscal**, consultar a venda e os
indicadores distintos de **Emissão automática** e **Consulta SEFAZ**,
além das tentativas e do diagnóstico de preflight. A tela do PDV
também mostra os estados básicos associados à venda atual.

Se houver `manual_review`, não repetir a venda, não apagar
arquivos XML, não zerar tentativas, não emitir automaticamente
outra nota e não fazer POST de autorização novamente com a mesma
operação. Registrar número, série, chave e estado da NFC-e,
consultar os serviços oficiais da SEF/MG com as credenciais
daquela **empresa/CNPJ específico**, e pedir avaliação do
responsável fiscal. Respostas como `101`, `110`, `204`, `217`
e `539` não devem disparar um novo envio automaticamente.

Para arquivo com hash incorreto, preservar os bytes e registros
do incidente para diagnóstico, restaurar **somente** material íntegro
e verificado a partir de um backup confiável; não alterar o
SHA-256 manualmente para fazer o sistema aceitar um arquivo
desconhecido.

## Evidências de testes sem dados reais

- PostgreSQL 16 descartável: aplicadas **48 migrações**.
- `TestAutomaticNFCeHomologationPlansOnlyNewConfiguredSales`:
  múltiplos trabalhadores com claims exclusivos, reenvio não
  permitido e última tentativa expirada escalada uma vez.
- `TestNFCeConsultationRecoveryIsTenantScopedAndFenced`:
  teste de Consulta sem envio, integridade do lease mesmo se
  expira durante a resposta, décima consulta expirada escalada,
  isolamento por CNPJ e idempotência.
- `TestAutoHomologationSaleToSubmittedToAuthorizedWithFakeSEFAZ`:
  venda fictícia → assinatura simulada → cStat inconclusivo →
  consulta simulada → autorização e nfeProc arquivado;
  recupera crash após envio sem autorizar novamente.
  Adulteração do hash desabilita impressão, download,
  status de autorização e conclusão automática; restauração
  de arquivo íntegro permite conclusão.
- Playwright Chromium `fiscal-obligations.spec.ts` e
  `pdv-fiscal-print-choice.spec.ts`: alertas de revisão
  manual/consulta, não impressão e proteção contra duplicidade
  de venda no comportamento simulado.
- `go test ./...`, `go vet ./...`, `npm run build`
  e `npm run lint`, sem GitHub Actions.

## Pendências reais / critérios para liberar

1. **Certificado e CNPJ reais de cada loja**, IE/CRT, cadastro
   cadastral completo e credenciamento pela SEF/MG. Cada loja
   usa sua própria empresa/tenant; não há modelo de filiais.
2. **Homologação efetiva:** SEFAZ real, certificado A1 válido,
   assinatura XML, cStat, consultas após timeout,
   duplicidades, rejeições, NFC-e 100, `nfeProc` oficial,
   cancelamento, inutilização e impressão física do DANFE.
3. **Tributação completa**, incluindo todas as hipóteses
   efetivas de IBS/CBS e imposto seletivo, revisão por
   profissional contábil e atualizações fiscais vigentes.
4. **NF-e modelo 55** necessita executor oficial separado;
   não deve ser considerado coberto pelo emissor NFC-e.
5. **Produção** só depois da validação de homologação,
   credenciamento e critérios técnicos e legais
   formalmente satisfeitos. Não ligar emissão de produção
   só porque os testes com mocks estão verdes.

A ausência de dados fiscais reais não impede desenvolver,
operar e testar o **PDV no navegador**, mas impede afirmar
que notas reais já podem ser emitidas legalmente.


## Vigilância local de integridade pós-autorização (15/10/2026)

O serviço que planeja consultas NFC-e executa periodicamente
`ReconcileNFCeArchivedIntegrity`, além das verificações de estados
terminais e tentativas esgotadas. Mesmo **depois** de uma fila ter sido
declarada `authorized` ou `resolved`, um documento com chave não
correspondente, protocolo ausente, `nfeProc` vazio ou SHA-256 divergente
faz o estado **local** dessa fila passar para `manual_review`.
A mesma defesa alcança filas ainda `queued`, `awaiting_sefaz` ou
`awaiting_consultation` se a nota já está marcada `authorized` mas
o arquivo processado não está íntegro.

A operação é transacional no PostgreSQL, verifica que nenhum lease
continua válido, não muda `invoices.status`, não inventa protocolo,
não elimina o conteúdo para perícia e **não realiza chamadas SOAP ou
transmissão SEFAZ**. O sistema emite um aviso técnico de quantos
trabalhos locais foram colocados em revisão, sem registrar o XML ou
dados pessoais no log. É possível repetir a verificação: sem alterações
novas, ela não modifica as mesmas filas novamente.

A rotina de planejamento `PlanNFCeSubmittedConsultations` também
descarta chaves que não estejam no formato de **44 dígitos numéricos
iniciados por 31 (MG)**, evitando que uma chave de outro estado ou
um dado legado malformado interrompa o processamento de todas as
consultas. Essa pré-filtragem não substitui a validação da chave
completa, executada antes de qualquer consulta remota.

O teste de integração cria, com dados **exclusivamente fictícios**, um
XML autorizado íntegro, modifica seu hash **após** a conclusão,
confirma a quarentena das filas e bloqueio das saídas fiscais,
exige idempotência, restaura a integridade e comprova a reconciliação.
Outro cenário mistura uma chave com UF incorreta e uma chave MG
válida: a incorreta é ignorada, sem atrasar o documento válido.

**Ação do operador:** uma fila marcada para revisão por corrupção é
um alerta de integridade, não uma rejeição SEFAZ. Preserve o original
e os registros, faça backup e solicite investigação; jamais tente
consertar o incidente criando um novo documento fiscal para a
mesma venda.
