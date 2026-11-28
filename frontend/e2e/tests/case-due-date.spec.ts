import { test, expect, type Page } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// The shared E2E workspace (config.test.toml) enables the built-in case due
// date with [case] due_date = true. This walks the whole journey: set it on
// create, see it on the detail page and in the list column, change it inline,
// clear it, and confirm every step survives a reload.

// The calendar day as the UI renders it (formatCalendarDate: UTC day in the
// browser's locale), computed in the page so the locale matches.
async function renderedDay(page: Page, day: string): Promise<string> {
  return page.evaluate(
    (d) => new Date(`${d}T00:00:00Z`).toLocaleDateString(undefined, { timeZone: 'UTC' }),
    day,
  );
}

function isUpdateCase(r: import('@playwright/test').Response): boolean {
  if (!r.url().includes('/graphql') || r.request().method() !== 'POST') return false;
  return r.request().postDataJSON?.()?.operationName === 'UpdateCase';
}

test.describe('Case built-in due date', () => {
  test('set on create, shown in detail and list, edited inline and cleared', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const title = `Sample due date case ${Date.now()}`;

    // Create with a due date through the form.
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title, customFields: { category: 'bug' }, dueDate: '2030-06-30' });

    // The list shows it in the Due Date column.
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    const row = caseListPage.getCaseRowByTitle(title);
    await expect(row.getByTestId('case-row-due-date')).toHaveText(await renderedDay(page, '2030-06-30'));

    // The detail page shows it in the sidebar.
    await caseListPage.clickCaseByTitle(title);
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();
    const dueDate = page.getByTestId('case-due-date');
    await expect(dueDate).toHaveText(await renderedDay(page, '2030-06-30'));

    // Change it inline; it round-trips through a reload.
    await dueDate.click();
    const changed = page.waitForResponse(isUpdateCase);
    const input = page.getByTestId('case-due-date-input');
    await input.fill('2030-07-31');
    await input.blur();
    expect((await changed).ok()).toBeTruthy();
    await page.reload();
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();
    await expect(page.getByTestId('case-due-date')).toHaveText(await renderedDay(page, '2030-07-31'));

    // Clear it inline; after a reload the placeholder is back.
    await page.getByTestId('case-due-date').click();
    const cleared = page.waitForResponse(isUpdateCase);
    const clearInput = page.getByTestId('case-due-date-input');
    await clearInput.fill('');
    await clearInput.blur();
    expect((await cleared).ok()).toBeTruthy();
    await page.reload();
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();
    await expect(page.getByTestId('case-due-date')).not.toHaveText(await renderedDay(page, '2030-07-31'));
    await expect(page.getByTestId('case-due-date-section')).toBeVisible();
  });
});
