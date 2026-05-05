# VueUse — сначала ищем готовый композабл

В зависимостях есть [`@vueuse/core`](https://vueuse.org/). Прежде чем писать ручную обёртку над `ref` + `watch` + DOM/Storage API, проверить, нет ли подходящего хука. Не плодить руками то, что vueuse уже покрывает.

## Типовые соответствия

| Что нужно | Используем |
|---|---|
| Boolean/enum toggle | `useToggle()` (вместо `const flag = ref(false); function toggleFlag() { flag.value = !flag.value }`) |
| Reactive ref, синхронизированный с `localStorage` | `useStorage()` (вместо `ref` + `watch` + `localStorage.setItem/getItem`) |
| Тёмная/светлая тема | `useColorMode()` / `useDark()` |
| Дебаунс/тротлинг ввода | `refDebounced`, `useDebounceFn`, `useThrottleFn` (вместо `setTimeout` руками) |
| `addEventListener` с автоочисткой | `useEventListener()` |
| Размер/позиция элемента | `useElementSize`, `useElementBounding`, `useResizeObserver` |
| Медиа-запросы | `useMediaQuery()` |
| Буфер обмена, share | `useClipboard`, `useShare` |
| Async data (если не TanStack Query) | `useAsyncState`, `useFetch` |

## Правила

- **Кастомный truthy/falsy в `useToggle`** — для перечислений из двух значений (`'light' | 'dark'`, `'asc' | 'desc'`):
  ```ts
  const [theme, toggleTheme] = useToggle<"dark", "light">("light", {
    truthyValue: "dark",
    falsyValue: "light",
  });
  ```
- **В шаблонах `@click` всегда зовём `toggle` со скобками** — `@click="toggle()"`, не `@click="toggle"`. Иначе Vue передаст `MouseEvent` как первый аргумент, и `toggle` интерпретирует его как явное значение состояния. Это поведение специально оговорено в документации vueuse.
- **Server-state остаётся в TanStack Query** — vueuse-композабл `useFetch` для запросов к нашему API не использовать; он для одноразовых случаев и не интегрируется с инвалидацией.
