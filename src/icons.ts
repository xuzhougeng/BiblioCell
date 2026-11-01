// Inline stroke icons used by the shell and every view.
const paths: Record<string, string> = {
  home: '<path d="M4 10.6a2 2 0 0 1 .7-1.5l6-5.1a2 2 0 0 1 2.6 0l6 5.1a2 2 0 0 1 .7 1.5v7.9a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z"/><path d="M9.5 20.5v-5a1.5 1.5 0 0 1 1.5-1.5h2a1.5 1.5 0 0 1 1.5 1.5v5"/>',
  book: '<path d="M12 6.6C10.4 5.1 8 4.5 4.8 4.5a1.3 1.3 0 0 0-1.3 1.3v11.4a1.3 1.3 0 0 0 1.3 1.3c3.2 0 5.6.6 7.2 2.1 1.6-1.5 4-2.1 7.2-2.1a1.3 1.3 0 0 0 1.3-1.3V5.8a1.3 1.3 0 0 0-1.3-1.3C16 4.5 13.6 5.1 12 6.6Z"/><path d="M12 6.6v14"/>',
  database: '<ellipse cx="12" cy="6" rx="7.75" ry="2.75"/><path d="M4.25 6v12a7.75 2.75 0 0 0 15.5 0V6"/><path d="M4.25 12a7.75 2.75 0 0 0 15.5 0"/>',
  check: '<path d="M9 5H7a2.5 2.5 0 0 0-2.5 2.5v11A2.5 2.5 0 0 0 7 21h10a2.5 2.5 0 0 0 2.5-2.5v-11A2.5 2.5 0 0 0 17 5h-2"/><rect x="9" y="3" width="6" height="4" rx="1.25"/><path d="m8.75 13.75 2.25 2.25 4.25-4.5"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 11h18"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  "arrow-right": '<path d="M4 12h16m-6-6 6 6-6 6"/>',
  "arrow-up-right": '<path d="M6 18 18 6M6 6h12v12"/>',
  "chevron-down": '<path d="m6 9 6 6 6-6"/>',
  "chevron-up": '<path d="m18 15-6-6-6 6"/>',
  "chevron-left": '<path d="m15 5-7 7 7 7"/>',
  "chevron-right": '<path d="m9 5 7 7-7 7"/>',
  filter: '<path d="M4 6h16M7 12h10m-7 6h4"/>',
  download: '<path d="M12 3v12m-5-5 5 5 5-5M4 15v6h16v-6"/>',
  file: '<path d="M14 2H5a1 1 0 0 0-1 1v18a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V8Z"/><path d="M14 2v6h6M8 13h8m-8 4h6"/>',
  link: '<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-2 2M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l2-2"/>',
  x: '<path d="m6 6 12 12M6 18 18 6"/>',
  "check-circle": '<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  more: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  star: '<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9Z"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M8.9 6Q9.9 5.4 10 4.3L10.1 4Q10.2 2.9 11.3 2.9L12.7 2.9Q13.8 2.9 13.9 4L14 4.3Q14.1 5.4 15.1 6L15.7 6.3Q16.6 6.9 17.6 6.5L18 6.3Q19 5.9 19.5 6.9L20.2 8Q20.7 9 19.9 9.7L19.6 9.9Q18.7 10.6 18.7 11.7L18.7 12.3Q18.7 13.4 19.6 14.1L19.9 14.3Q20.7 15 20.2 16L19.5 17.1Q19 18.1 18 17.7L17.6 17.5Q16.6 17.1 15.7 17.7L15.1 18Q14.1 18.6 14 19.7L13.9 20Q13.8 21.1 12.7 21.1L11.3 21.1Q10.2 21.1 10.1 20L10 19.7Q9.9 18.6 8.9 18L8.3 17.7Q7.4 17.1 6.4 17.5L6 17.7Q5 18.1 4.5 17.1L3.8 16Q3.3 15 4.1 14.3L4.4 14.1Q5.3 13.4 5.3 12.3L5.3 11.7Q5.3 10.6 4.4 9.9L4.1 9.7Q3.3 9 3.8 8L4.5 6.9Q5 5.9 6 6.3L6.4 6.5Q7.4 6.9 8.3 6.3Z"/>',
  leaf: '<path d="M20 4c-7-2-15 1-15 8a7 7 0 0 0 7 7c7 0 9-8 8-15Z"/><path d="m4 21 11-11M9 16v-5m0 5h5"/>',
  refresh: '<path d="M4 12a8 8 0 0 1 14-5.3L20 8.5"/><path d="M20 3.75V8.5h-4.75"/><path d="M20 12a8 8 0 0 1-14 5.3L4 15.5"/><path d="M4 20.25V15.5h4.75"/>',
  grid: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  schema: '<rect x="8.5" y="3.5" width="7" height="5.5" rx="1.5"/><rect x="3.5" y="15" width="7" height="5.5" rx="1.5"/><rect x="13.5" y="15" width="7" height="5.5" rx="1.5"/><path d="M12 9v3M7 15v-1.5A1.5 1.5 0 0 1 8.5 12h7a1.5 1.5 0 0 1 1.5 1.5V15"/>',
  code: '<path d="m8 5-7 7 7 7m8-14 7 7-7 7m-3-17-2 20"/>',
  folder: '<path d="M3 5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
  upload: '<path d="M12 16V3m-5 5 5-5 5 5M4 16v5h16v-5"/>',
  alert: '<path d="m12 3 10 18H2Z"/><path d="M12 9v5m0 3v.1"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v.1"/>',
  trash: '<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>',
  sidebar: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16M5.5 8h1.5M5.5 11h1.5"/>',
  // GitHub 的品牌标志是填充图形，自带 fill/stroke 覆盖描边默认值。
  github: '<g transform="translate(4 4)"><path fill="currentColor" stroke="none" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></g>',
};

/** icon returns an inline SVG; unknown names fall back to the file glyph. */
export function icon(name: string, size = 18): string {
  const px = Number(size) || 18;
  return `<svg width="${px}" height="${px}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] ?? paths.file ?? ""}</svg>`;
}
