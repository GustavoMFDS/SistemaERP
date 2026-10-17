# Esterium Local — HTTPS obrigatório para interface na LAN (L3 parcial)

A interface pré-compilada pode ser servida pelo executável Go (L0). A partir desta mudança, o servidor **recusa a inicialização** quando `ESTERIUM_WEB_DIR` estiver definido e o endereço `HTTP_ADDR` estiver exposto na rede sem TLS.

O objetivo é evitar transmitir autenticação e dados comerciais em HTTP sem proteção e garantir contexto seguro para os recursos PWA utilizados pelos caixas.

## Configuração no ambiente de homologação

- `ESTERIUM_WEB_DIR`: pasta do frontend compilado com `index.html`.
- `HTTP_ADDR`: endereço e porta do servidor, por exemplo `:8443`.
- `ESTERIUM_TLS_CERT_FILE`: caminho absoluto do certificado do servidor com cadeia apropriada.
- `ESTERIUM_TLS_KEY_FILE`: caminho absoluto da chave privada com acesso exclusivo à conta do serviço.
- Utiliza TLS 1.3 no listener e verifica previamente se certificado/chave coincidem e se as datas de validade são atuais.

O operador deve acessar uma URL **HTTPS estável** associada ao nome do equipamento/loja, usando um certificado confiável para os navegadores dos demais terminais. A distribuição de confiança do certificado não é automatizada por este PR. Um certificado válido no lado do servidor, isoladamente, não prova que o navegador confia nele.

Sem certificado, a SPA só pode ser iniciada em `localhost`, `127.0.0.1` ou `::1`. A API-only conserva o comportamento anterior para instalações que empregam proxy HTTPS externo.

## Segurança e limites

1. A chave privada **não pode ser copiada** para os terminais da loja nem incluída no pacote compilado. Deve ficar sob ACL restrita ao serviço local.
2. O endereço do backend não deve ser roteado para a Internet pública. O firewall deve permitir apenas LAN privada e a porta de HTTPS configurada.
3. Banco PostgreSQL e backend de sessões devem permanecer em loopback, sem exposição LAN.
4. Certificados de teste/autossinados gerados pelo CI **não são certificados de produção**. Instalação, renovação e revogação da confiança nos terminais precisam de solução própria.
5. Testar acesso por pelo menos dois caixas Windows/Chrome/Firefox, expiração de certificado, troca de endereço IP, reboot, impressoras e PWA real.
6. Fiscal: autorização NF-e/NFC-e SEFAZ continua bloqueada, independentemente do HTTPS interno.

**Não é um instalador completo:** provisionamento PostgreSQL/Redis, geração/instalação de confiança TLS e backup externo restaurado permanecem gates de produção.
