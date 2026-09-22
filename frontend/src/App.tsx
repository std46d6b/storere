import { useMemo, useState } from 'react'

const demoItems = [
  { name: 'Синие лыжные перчатки', box: 'Зимняя одежда', location: 'Балкон', tag: 'спорт', photos: 1 },
  { name: 'Запасные лампочки E27', box: 'Ремонт', location: 'Прихожая', tag: 'дом', photos: 2 },
  { name: 'Кабель USB-C 2 м', box: 'Электроника', location: 'Рабочий стол', tag: 'техника', photos: 1 }
]

export function App() {
  const [query, setQuery] = useState('')
  const [dialog, setDialog] = useState(false)
  const [dark, setDark] = useState(true)
  const items = useMemo(() => demoItems.filter((x) => `${x.name} ${x.box} ${x.tag}`.toLowerCase().includes(query.toLowerCase())), [query])
  return <main className={dark ? 'app dark' : 'app'}>
    <aside><strong>storere</strong><nav><button className="active">⌕ Поиск</button><button>▣ Коробки</button><button>⌖ Места</button><button>◷ История</button></nav><small>Квартира на Лесной</small></aside>
    <section className="content">
      <header><div><p className="eyebrow">ИНВЕНТАРЬ</p><h1>Мои вещи</h1></div><div className="actions"><button aria-label="Переключить тему" onClick={() => setDark(!dark)}>{dark ? '☀' : '☾'}</button><button className="primary" onClick={() => setDialog(true)}>＋ Добавить</button></div></header>
      <label className="search"><span>⌕</span><input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Найти вещь, коробку или тег…" autoFocus /></label>
      <div className="filters"><button>Все вещи</button><button>Коробки</button><button>Теги</button><button>Без коробки</button></div>
      <div className="summary"><span>{items.length} вещи</span><button>Недавно изменённые ▾</button></div>
      <div className="grid">{items.map((item) => <article key={item.name}><div className="preview" aria-label={`Фото: ${item.name}`}>▧<span>{item.photos} фото</span></div><div className="card"><div><h2>{item.name}</h2><p>{item.box} · {item.location}</p></div><em>{item.tag}</em></div></article>)}</div>
      {items.length === 0 && <div className="empty">Ничего не найдено. Попробуйте другое слово или добавьте вещь.</div>}
    </section>
    <button className="fab" aria-label="Быстрое создание" onClick={() => setDialog(true)}>＋</button>
    {dialog && <div className="backdrop" role="dialog" aria-modal="true" aria-label="Добавить"><div className="dialog"><button className="close" onClick={() => setDialog(false)} aria-label="Закрыть">×</button><p className="eyebrow">БЫСТРОЕ ДОБАВЛЕНИЕ</p><h2>Что добавить?</h2><button className="choice">▧ <span><b>Вещь</b><small>Фото обязательно</small></span></button><button className="choice">□ <span><b>Коробку</b><small>Контейнер для вещей</small></span></button><button className="choice">⌖ <span><b>Место</b><small>Балкон, полка, комната</small></span></button></div></div>}
  </main>
}
