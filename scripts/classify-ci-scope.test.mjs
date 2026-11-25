import { test } from 'node:test';
import assert from 'node:assert/strict';
import { classify } from './classify-ci-scope.mjs';
const d='docs/esterium-2/22-foundation.md';
const w='.github/workflows/esterium-windows-setup.yml';
const t='web/smoke/installed-lab.spec.ts';
for (const [name, paths, event, expected] of [
 ['docs only',[d],'pull_request','docs'],
 ['Windows workflow only',[w],'pull_request','windows'],
 ['installed browser fixture',[t],'pull_request','windows'],
 ['Windows fixture plus docs',[t,d],'pull_request','windows'],
 ['backend plus docs',['backend/cmd/api/main.go',d],'pull_request','full'],
 ['frontend source',['web/src/App.tsx'],'pull_request','full'],
 ['SQL',['backend/migrations/0055_example.up.sql'],'pull_request','full'],
 ['other workflow',['.github/workflows/ci.yml'],'pull_request','full'],
 ['classifier change',['scripts/classify-ci-scope.mjs'],'pull_request','full'],
 ['other docs',['docs/database-least-privilege.md'],'pull_request','full'],
 ['unknown',['new-file'],'pull_request','full'],
 ['rename from source',['backend/main.go',d],'pull_request','full'],
 ['empty',[],'pull_request','full'],
 ['invalid path',['docs/esterium-2/../foo.md'],'pull_request','full'],
 ['push',[d],'push','full'],
 ['dispatch',[w],'workflow_dispatch','full'],
]) test(name,()=>assert.equal(classify(paths,event),expected));
