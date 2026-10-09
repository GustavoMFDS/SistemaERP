-- Schema-level regression for the private product gallery.
-- Run against an isolated migrated PostgreSQL database; all fixtures roll back.
\set ON_ERROR_STOP on
BEGIN;
DO $test$
DECLARE
 a uuid;
 b uuid;
 pa uuid;
 pb uuid;
 ia uuid;
 key_a uuid := gen_random_uuid();
 base_hash bytea := decode(repeat('00',32),'hex');
 photo bytea := decode('abcd','hex');
 n integer;
BEGIN
 INSERT INTO companies (legal_name, cnpj)
 VALUES ('Media Test Shop A', lpad((floor(random()*100000000000000))::bigint::text,14,'0'))
 RETURNING id INTO a;
 INSERT INTO companies (legal_name, cnpj)
 VALUES ('Media Test Shop B', lpad((floor(random()*100000000000000))::bigint::text,14,'0'))
 RETURNING id INTO b;
 INSERT INTO products (tenant_id,sku,name,unit,price_cash)
 VALUES (a,'MEDIA-CHECK-A-'||gen_random_uuid()::text,'Caderno Azul','un',9.90)
 RETURNING id INTO pa;
 INSERT INTO products (tenant_id,sku,name,unit,price_cash)
 VALUES (b,'MEDIA-CHECK-B-'||gen_random_uuid()::text,'Caderno Rosa','un',9.90)
 RETURNING id INTO pb;
 INSERT INTO product_images(tenant_id,product_id,upload_key,content_sha256,image_data,thumb_data)
 VALUES (a,pa,key_a,base_hash,photo,photo) RETURNING id INTO ia;
 SELECT count(*) INTO n FROM product_images WHERE tenant_id=a AND product_id=pa;
 IF n <> 1 THEN RAISE EXCEPTION 'A expected one photo, got %',n; END IF;
 SELECT count(*) INTO n FROM product_images WHERE tenant_id=b AND product_id=pa;
 IF n <> 0 THEN RAISE EXCEPTION 'Tenant B could see photo A'; END IF;
 BEGIN
   INSERT INTO product_images(tenant_id,product_id,upload_key,content_sha256,image_data,thumb_data)
   VALUES (a,pa,key_a,base_hash,photo,photo);
   RAISE EXCEPTION 'Duplicate upload key accepted';
 EXCEPTION WHEN unique_violation THEN NULL;
 END;
 BEGIN
   INSERT INTO product_images(tenant_id,product_id,upload_key,content_sha256,image_data,thumb_data)
   VALUES (b,pa,gen_random_uuid(),base_hash,photo,photo);
   RAISE EXCEPTION 'Cross-tenant product photo accepted';
 EXCEPTION WHEN foreign_key_violation THEN NULL;
 END;
 INSERT INTO product_images(tenant_id,product_id,upload_key,content_sha256,image_data,thumb_data)
 VALUES (b,pb,gen_random_uuid(),base_hash,photo,photo);
 RAISE NOTICE 'PASS: independent shops, duplicate-key and composite FK';
END $test$;
ROLLBACK;
