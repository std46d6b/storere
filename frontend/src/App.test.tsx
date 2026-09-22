import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from './App'

describe('App', () => {
  it('shows inventory search and an always available add control', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: /мои вещи/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /добавить/i })).toBeInTheDocument()
  })
})
