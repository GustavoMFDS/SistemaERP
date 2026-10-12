# SistemaEmGo — processamento automático de NFC-e em MG (12/10/2026)

## O que é possível sem dados reais

O PDV pode registrar vendas, baixar estoque, guardar pagamentos, controlar o caixa
e criar `sale_fiscal_intents` para **todas** as vendas mesmo sem CNPJ/certificado.
Uma venda concluída **não** é documento fiscal emitido. Enquanto a autorização
da SEF/MG não existir com XML processado, a interface mostra a pendência e
não habilita impressão de DANFE como documento autorizado.

O fluxo de impressão permanece semi-automático: após autorizar, o operador
pode optar por **imprimir ou não imprimir**; desistir da impressão não modifica
a venda, o registro fiscal ou a obrigação de emissão.

## Ativação de integração em homologação

A comunicação com a SEF/MG existe no provider `sefaz`, utilizando certificado
digital, TLS mútuo, XML assinado, esquema XSD e endereços de serviço de MG.
Os URLs oficiais ficam em
https://portalsped.fazenda.mg.gov.br/spedmg/nfce/web-services/ .
O código não usa login/senha do portal SIARE para executar o SOAP da NFC-e.

**Ambiente de desenvolvimento/demo:** nenhum acesso real à SEFAZ é realizado.
Os flags de transmissão/automação iniciam em `false`, inclusive se o usuário
preencher a página Fiscal. Assim, pode-se testar o PDV agora sem dados reais.

**Quando houver certificado e homologação preparada**, as opções de deploy
necessárias para habilitar o fluxo automatizado de **homologação** são:

```dotenv
APP_ENV=staging
FISCAL_PROVIDER=sefaz
NFCE_SEFAZ_HOMOLOGATION_ENABLED=true
NFCE_AUTO_HOMOLOGATION_ENABLED=true

NFCE_CERTIFICATE_SECRET_DIR=/caminho/seguro/nao-publico
NFCE_SCHEMA_DIR=/caminho/bundle-oficial-XSD
NFCE_SCHEMA_ENTRYPOINT=leiauteNFe_v4.00.xsd
NFCE_EVENT_SCHEMA_ENTRYPOINT=envEventoCancNFe_v1.00.xsd
NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT=inutNFe_v4.00.xsd
```

Esses valores indicam **como preparar uma instalação futura**, não que seja
seguro ligá-los sem certificado, XSD e autorização de homologação reais. O
backend **recusa inicialização** da automação se os componentes necessários
não puderem ser construídos. A senha do PFX não deve ser digitada em campos
cadastrais nem salva em texto na base. O resolvedor atual usa material PEM
provisionado fora do banco, referenciado por empresa; jamais comitar as chaves.

Na página Fiscal da loja, o administrador também deverá cadastrar, com
orientação contábil, CNPJ/razão social, IE, CRT verdadeiro, município IBGE MG,
CEP, série de NFC-e, referência do certificado e perfis de tributação de cada
produto. Não presumir CRT a partir de `não sou MEI`. Cada loja possui seu
**próprio tenant/CNPJ**, e nenhuma configuração pode ser compartilhada
inadvertidamente com outra empresa.

A automação configurada acima **só atua em homologação**. Para produção,
continuam obrigatórios autorização legal da empresa, credenciamento,
certificado, fiscalidade válida por operação e validação efetiva do executor.
`NFCE_SEFAZ_PRODUCTION_ENABLED` não ativa a fila automática deste documento.

## Máquina de estados

1. Venda finalizada: gatilho do PostgreSQL gera obrigação fiscal, independente
   da opção de impressão.
2. `PlanNFCeAutomaticHomologation` seleciona apenas **vendas novas**
   (`finalized_at >= nfce_configs.updated_at`), não legadas, modelo `nfce`,
   com certificado referenciado e ambiente MG/homologação. Nunca reemite
   vendas antigas ao ativar a automação.
3. `nfce_auto_homologation_jobs` (migração 0047) garante planejamento
   idempotente por empresa/venda, lease exclusivo, identidade técnica ativa,
   no máximo 12 tentativas e retorno explícito `manual_review`.
4. `ProcessNFCeAutoHomologationClaim` executa preflight estrutural,
   reserva número exatamente uma vez no banco, gera snapshots fiscais,
   arquiva **somente cálculos legados explicitamente configurados** para os
   itens, assina XML com o A1 e valida XML assinado contra XSD instalado.
   Nunca adivinha alíquota IBS/CBS, imposto seletivo ou CRT; se um desses
   cálculos for necessário e não estiver implementado/configurado, a venda
   permanece pendente para revisão, não se transmite XML falso.
5. Em `signed`, `AuthorizeNFCeHomologation` grava `submitted` e auditoria
   **antes** do SOAP. Qualquer timeout posterior permanece `submitted`:
   jamais repete autorização às cegas. Reintentos só consultam chave.
6. `nfce_consult_recovery_jobs` (migração 0046) possui executor consultivo
   com leases separados, limite de 10 tentativas e verificação de operador,
   empresa, ambiente, certificado, chave, código SEFAZ e protocolo XML.
   Um retorno genuíno 100 pode arquivar status, protocolo, `nfeProc` e
   auditoria em **uma só transação**, com verificação do lease antes do commit.
7. `ObserveAuthorizedAutomaticNFCeJob` só sinaliza a fila como autorizada
   se existe nota realmente `authorized`, protocolo/data e XML processado
   com chave correspondente. O próprio `GET /sales/{id}/fiscal-status`
   também só habilita impressão mediante arquivo de XML processado.
8. A tela do PDV consulta automaticamente o status de 5 em 5 segundos após
   a venda, sem repetir venda nem disparar impressão. O cliente decide se
   quer receber DANFE e o operador imprime apenas quando houver autorização.

### Segurança e limites operacionais

- `NFCE_AUTO_HOMOLOGATION_ENABLED` é **false** por padrão, requer
  `NFCE_SEFAZ_HOMOLOGATION_ENABLED=true`, `FISCAL_PROVIDER=sefaz`,
  `APP_ENV` não produção, certificado e esquema. São opções de
  implantação, **não** campos que um caixa possa modificar.
- A automação não processa NF-e modelo 55: essas vendas permanecem para
  fluxo de NF-e separado, que ainda não está concluído.
- Códigos SEFAZ `217` (não consta), `101`, `110`, `204`, `539`
  nunca habilitam reenvio ou reemissão automática. Escalar ao operador.
- `manual_review` requer intervenção/reconciliação individual; não se
  pode apagar a fila ou baixar o histórico para forçar emissão.
- Cadastro em conformidade local **não prova** credenciamento SIARE,
  validade de IE, correta classificação tributária ou aprovação na SEF/MG.
- Para certas atividades (por exemplo, comércio de combustíveis com
  CNAE 4731-8), há obrigações adicionais de responsável técnico; consultar
  as normas vigentes e o contador antes da operação.

## Testes e restrições de aceite

Os testes de integração criam vendas e empresas **fictícias**, usam PostgreSQL
descartável e simulam o provider SEFAZ. São capazes de provar transições,
concorrência/idempotência de filas, ausência de reenvio e atomicidade de XML,
mas **não** comprovam homologação real, validade jurídica, documento XSD
oficial com certificado da empresa nem disponibilidade real da SEF/MG.

Antes de habilitar NFC-e real em **produção**, realizar com CNPJ, certificado
A1 válido e contador os testes de homologação oficial (incluindo 100, 103,
104, rejeições, duplicidades, perda de conexão, contingência, cancelamento,
inutilização, download do `nfeProc` e DANFE). A integração automática de
produção e o cálculo completo por regime/RTC continuam pendentes: **não**
declarar essa etapa como concluída só porque a aplicação compila.

Não executar GitHub Actions para essa validação; usar banco descartável,
testes Go e navegador local.
