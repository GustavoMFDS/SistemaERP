import { test, expect } from '@playwright/test'
test('fiscal page rendering diagnostic',async({page})=>{
 const errors:string[]=[];
 page.on('pageerror',error=>errors.push(error.stack||error.message));
 await page.goto('/login');
 await page.getByLabel('E-mail').fill('admin@sistema.local');
 await page.getByLabel('Senha').fill('admin123');
 await page.getByRole('button',{name:'Entrar'}).click();
 await page.getByRole('link',{name:/Fiscal/}).click();
 await expect(page.getByRole('heading',{name:/Fiscal/})).toBeVisible();
 await expect(page.getByText('Prontidão para homologação')).toBeVisible();
 expect(errors).toEqual([]);
});
