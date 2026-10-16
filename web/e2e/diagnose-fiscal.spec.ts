import { test, expect } from './fixtures'
test('fiscal page rendering diagnostic',async({page})=>{
 const errors:string[]=[];
 page.on('pageerror',error=>errors.push(error.stack||error.message));
 await page.goto('/login');
 await page.getByLabel('E-mail').fill('admin@sistema.local');
 await page.getByLabel('Senha').fill('admin123');
 await page.getByRole('button',{name:'Entrar'}).click();
 await page.getByRole('navigation', { name: 'Navegação principal' }).getByText('Mais áreas do sistema').click();
 await page.getByRole('link',{name:'Fiscal', exact: true}).click();
 await expect(page.getByRole('heading',{name:'Nota fiscal da loja'})).toBeVisible();
 await expect(page.getByRole('heading', { name: 'Diagnóstico local da configuração fiscal' })).toBeVisible();
 expect(errors).toEqual([]);
});
