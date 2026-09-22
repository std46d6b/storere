import { FormEvent, useMemo, useState } from 'react'
import {
  ApiError, Box, Item, Location, Media, Space,
  useBoxesQuery, useCreateBoxMutation, useCreateItemMutation, useCreateLocationMutation,
  useCreateSpaceMutation, useItemsQuery, useLocationsQuery, useLoginMutation,
  useLogoutMutation, useMeQuery, useRegisterMutation, useSearchQuery, useSpacesQuery,
  useUploadMediaMutation
} from './api/api'

function message(error: unknown) {
  const value = error as { data?: ApiError }
  return value?.data?.message ?? 'Не удалось выполнить запрос. Попробуйте ещё раз.'
}

function AuthPage() {
  const [registering, setRegistering] = useState(false)
  const [login, { isLoading: loggingIn, error: loginError }] = useLoginMutation()
  const [register, { isLoading: registeringUser, error: registerError }] = useRegisterMutation()
  const pending = loggingIn || registeringUser
  const error = loginError ?? registerError

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const username = String(data.get('username') ?? '')
    const password = String(data.get('password') ?? '')
    if (registering) await register({ username, password, displayName: String(data.get('displayName') ?? '') }).unwrap()
    else await login({ username, password }).unwrap()
  }

  return <main className="auth-page">
    <section className="auth-card" aria-labelledby="auth-title">
      <strong className="brand">storere</strong>
      <p className="eyebrow">ЛИЧНЫЙ ИНВЕНТАРЬ</p>
      <h1 id="auth-title">{registering ? 'Создать аккаунт' : 'Войти'}</h1>
      <p className="muted">Храните вещи, коробки и места в одном понятном каталоге.</p>
      <form onSubmit={submit}>
        {registering && <label>Отображаемое имя<input name="displayName" required minLength={1} autoComplete="name" /></label>}
        <label>Имя пользователя<input name="username" required minLength={3} autoComplete="username" /></label>
        <label>Пароль<input name="password" required minLength={12} type="password" autoComplete={registering ? 'new-password' : 'current-password'} /></label>
        {error && <p role="alert" className="form-error">{message(error)}</p>}
        <button className="primary wide" disabled={pending}>{pending ? 'Подождите…' : registering ? 'Зарегистрироваться' : 'Войти'}</button>
      </form>
      <button className="link-button" onClick={() => setRegistering((value) => !value)}>{registering ? 'У меня уже есть аккаунт' : 'Создать аккаунт'}</button>
    </section>
  </main>
}

function SpaceSetup() {
  const [createSpace, state] = useCreateSpaceMutation()
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    await createSpace({ name: String(data.get('name')), description: String(data.get('description') || ''), address: String(data.get('address') || '') }).unwrap()
  }
  return <section className="empty setup"><p className="eyebrow">ПЕРВОЕ ПРОСТРАНСТВО</p><h1>Где находятся ваши вещи?</h1><p>Например, «Квартира на Лесной» или «Дача».</p><form onSubmit={submit}><label>Название<input name="name" required autoFocus /></label><label>Описание<textarea name="description" /></label><label>Адрес <span className="muted">(необязательно)</span><input name="address" /></label>{state.error && <p role="alert" className="form-error">{message(state.error)}</p>}<button className="primary" disabled={state.isLoading}>Создать пространство</button></form></section>
}

function AddDialog({ space, onClose }: { space: Space; onClose: () => void }) {
  const [kind, setKind] = useState<'menu' | 'item' | 'box' | 'location'>('menu')
  const [uploadMedia, uploadState] = useUploadMediaMutation()
  const [createItem, itemState] = useCreateItemMutation()
  const [createBox, boxState] = useCreateBoxMutation()
  const [createLocation, locationState] = useCreateLocationMutation()
  const { data: locations = [] } = useLocationsQuery(space.id)
  const { data: boxes = [] } = useBoxesQuery(space.id)
  const [media, setMedia] = useState<Media | null>(null)

  async function upload(file?: File) {
    if (!file) return
    setMedia(await uploadMedia({ spaceId: space.id, file }).unwrap())
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    if (kind === 'location') await createLocation({ spaceId: space.id, name: String(data.get('name')), code: String(data.get('code') || ''), description: String(data.get('description') || '') }).unwrap()
    if (kind === 'box') await createBox({ spaceId: space.id, name: String(data.get('name')), description: String(data.get('description') || ''), locationId: String(data.get('locationId') || '') }).unwrap()
    if (kind === 'item' && media) await createItem({ spaceId: space.id, name: String(data.get('name')), description: String(data.get('description') || ''), boxId: String(data.get('boxId') || ''), mediaIds: [media.id] }).unwrap()
    onClose()
  }
  const mutation = kind === 'item' ? itemState : kind === 'box' ? boxState : locationState
  return <div className="backdrop" role="presentation"><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="add-title"><button className="close" onClick={onClose} aria-label="Закрыть">×</button>{kind === 'menu' ? <><p className="eyebrow">БЫСТРОЕ ДОБАВЛЕНИЕ</p><h2 id="add-title">Что добавить?</h2><button className="choice" onClick={() => setKind('item')}>▧ <span><b>Вещь</b><small>Фото обязательно</small></span></button><button className="choice" onClick={() => setKind('box')}>□ <span><b>Коробку</b><small>Контейнер для вещей</small></span></button><button className="choice" onClick={() => setKind('location')}>⌖ <span><b>Место</b><small>Балкон, полка, комната</small></span></button></> : <form onSubmit={submit}><p className="eyebrow">{kind === 'item' ? 'НОВАЯ ВЕЩЬ' : kind === 'box' ? 'НОВАЯ КОРОБКА' : 'НОВОЕ МЕСТО'}</p><h2 id="add-title">Добавить</h2><label>Название<input name="name" required autoFocus /></label><label>Описание<textarea name="description" /></label>{kind === 'location' && <label>Код<input name="code" placeholder="kitchen" /></label>}{kind === 'box' && <label>Место<select name="locationId"><option value="">Не указано</option>{locations.map((location: Location) => <option key={location.id} value={location.id}>{location.name}</option>)}</select></label>}{kind === 'item' && <><label>Коробка<select name="boxId"><option value="">Без коробки</option>{boxes.map((box: Box) => <option key={box.id} value={box.id}>{box.name}</option>)}</select></label><label>Фотография<input aria-label="Фотография" type="file" accept="image/jpeg,image/png,image/webp" required={!media} onChange={(event) => upload(event.currentTarget.files?.[0])} /></label>{uploadState.error && <p role="alert" className="form-error">{message(uploadState.error)}</p>}{media && <img className="upload-preview" src={media.url} alt="Загруженное фото" />}</>}{mutation.error && <p role="alert" className="form-error">{message(mutation.error)}</p>}<button className="primary" disabled={mutation.isLoading || uploadState.isLoading || (kind === 'item' && !media)}>{uploadState.isLoading ? 'Загрузка…' : 'Сохранить'}</button></form>}</section></div>
}

function Inventory({ user }: { user: { username: string; displayName: string } }) {
  const { data: spaces = [], isLoading: spacesLoading, error: spacesError } = useSpacesQuery()
  const [selectedSpaceId, setSelectedSpaceId] = useState('')
  const activeSpace = spaces.find((space) => space.id === selectedSpaceId) ?? spaces[0]
  const [query, setQuery] = useState('')
  const [dark, setDark] = useState(true)
  const [dialog, setDialog] = useState(false)
  const [view, setView] = useState<'items' | 'boxes' | 'locations'>('items')
  const [logout] = useLogoutMutation()
  const { data: itemList = [], isLoading: itemsLoading, error: itemsError } = useItemsQuery(activeSpace?.id ?? '', { skip: !activeSpace || Boolean(query) })
  const { data: searchResults = [] } = useSearchQuery({ spaceId: activeSpace?.id ?? '', q: query }, { skip: !activeSpace || !query })
  const { data: boxList = [] } = useBoxesQuery(activeSpace?.id ?? '', { skip: !activeSpace })
  const { data: locationList = [] } = useLocationsQuery(activeSpace?.id ?? '', { skip: !activeSpace })
  const items = query ? searchResults : itemList
  const content = view === 'items' ? items : view === 'boxes' ? boxList : locationList

  if (spacesLoading) return <main className="auth-page"><p role="status">Загружаем пространства…</p></main>
  if (spacesError) return <main className="auth-page"><p role="alert">{message(spacesError)}</p></main>
  if (!activeSpace) return <main className={dark ? 'app dark' : 'app'}><aside><strong>storere</strong></aside><SpaceSetup /></main>

  return <main className={dark ? 'app dark' : 'app'}><aside><strong>storere</strong><nav><button className={view === 'items' ? 'active' : ''} onClick={() => setView('items')}>⌕ Вещи</button><button className={view === 'boxes' ? 'active' : ''} onClick={() => setView('boxes')}>▣ Коробки</button><button className={view === 'locations' ? 'active' : ''} onClick={() => setView('locations')}>⌖ Места</button></nav><label className="space-switcher">Пространство<select value={activeSpace.id} onChange={(event) => setSelectedSpaceId(event.target.value)}>{spaces.map((space) => <option key={space.id} value={space.id}>{space.name}</option>)}</select></label><small>{user.displayName} · @{user.username}<button className="link-button" onClick={() => logout()}>Выйти</button></small></aside><section className="content"><header><div><p className="eyebrow">{activeSpace.name.toUpperCase()}</p><h1>{view === 'items' ? 'Мои вещи' : view === 'boxes' ? 'Коробки' : 'Места'}</h1></div><div className="actions"><button aria-label="Переключить тему" onClick={() => setDark(!dark)}>{dark ? '☀' : '☾'}</button><button className="primary" onClick={() => setDialog(true)}>＋ Добавить</button></div></header>{view === 'items' && <label className="search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Найти вещь, коробку или тег…" autoFocus /></label>}{itemsError && <p role="alert" className="form-error">{message(itemsError)}</p>}<div className="summary"><span>{content.length} {view === 'items' ? 'вещи' : view === 'boxes' ? 'коробки' : 'места'}</span></div>{(itemsLoading && view === 'items') ? <p role="status">Загружаем вещи…</p> : <div className="grid">{view === 'items' ? (items as Item[]).map((item) => <article key={item.id}><div className="preview">{item.media?.[0] ? <img src={item.media[0].url} alt={item.name} /> : '▧'}<span>{item.photoCount} фото</span></div><div className="card"><div><h2>{item.name}</h2><p>{item.boxName ?? 'Без коробки'}{item.locationName ? ` · ${item.locationName}` : ''}</p></div>{item.tags?.[0] && <em>{item.tags[0].name}</em>}</div></article>) : (content as Array<Box | Location>).map((entry) => <article key={entry.id}><div className="preview">{view === 'boxes' ? '□' : '⌖'}</div><div className="card"><div><h2>{entry.name}</h2><p>{'itemCount' in entry ? `${entry.itemCount} вещей` : entry.description || 'Место хранения'}</p></div></div></article>)}</div>}{content.length === 0 && <div className="empty">Пока ничего нет. Добавьте первую запись.</div>}</section><button className="fab" aria-label="Быстрое создание" onClick={() => setDialog(true)}>＋</button>{dialog && <AddDialog space={activeSpace} onClose={() => setDialog(false)} />}</main>
}

export function App() {
  const session = useMeQuery()
  if (session.isLoading) return <main className="auth-page"><p role="status">Проверяем сессию…</p></main>
  if (!session.data) return <AuthPage />
  return <Inventory user={session.data} />
}
