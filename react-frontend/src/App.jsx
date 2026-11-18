import { useCallback, useEffect, useState } from 'react'
import { api } from './api'

const emptyForm = { name: '', email: '' }

export default function App() {
  const [users, setUsers] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(null)

  const loadUsers = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setUsers(await api.list())
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    api.health()
      .then(() => setConnected(true))
      .catch(() => setConnected(false))
    loadUsers()
  }, [loadUsers])

  const submit = async (e) => {
    e.preventDefault()
    if (!form.name.trim()) {
      setError('姓名不能为空')
      return
    }
    setSaving(true)
    setError('')
    try {
      if (editingId === null) {
        await api.create({ name: form.name, email: form.email })
      } else {
        await api.patch(editingId, { name: form.name, email: form.email })
      }
      setForm(emptyForm)
      setEditingId(null)
      await loadUsers()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const edit = (user) => {
    setEditingId(user.id)
    setForm({ name: user.name, email: user.email })
    setError('')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const cancelEdit = () => {
    setEditingId(null)
    setForm(emptyForm)
    setError('')
  }

  const remove = async (id) => {
    if (!window.confirm(`确定删除 ID 为 ${id} 的用户吗？`)) return
    setError('')
    try {
      await api.remove(id)
      if (editingId === id) cancelEdit()
      await loadUsers()
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <div className="page">
      <header className="header">
        <div>
          <h1>用户管理</h1>
          <p>React 前端调用 Go REST API（localhost:8080）</p>
        </div>
        <span className={`status ${connected === true ? 'ok' : connected === false ? 'bad' : ''}`}>
          {connected === null ? '检测中…' : connected ? '● 服务正常' : '● 服务不可用'}
        </span>
      </header>

      {error && <div className="alert">{error}</div>}

      <section className="card">
        <h2>{editingId === null ? '新增用户' : `编辑用户 #${editingId}`}</h2>
        <form onSubmit={submit}>
          <div className="row">
            <label>
              姓名
              <input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="例如：张三"
              />
            </label>
            <label>
              邮箱
              <input
                type="email"
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                placeholder="例如：zhangsan@example.com"
              />
            </label>
          </div>
          <div className="actions">
            <button type="submit" disabled={saving}>
              {saving ? '保存中…' : editingId === null ? '创建' : '保存修改'}
            </button>
            {editingId !== null && (
              <button type="button" className="secondary" onClick={cancelEdit}>
                取消
              </button>
            )}
            <button type="button" className="ghost" onClick={loadUsers} disabled={loading}>
              刷新列表
            </button>
          </div>
        </form>
      </section>

      <section className="card">
        <h2>用户列表 <span className="count">{users.length}</span></h2>
        {loading ? (
          <p className="muted">加载中…</p>
        ) : users.length === 0 ? (
          <p className="muted">暂无用户，先在上方创建一个吧。</p>
        ) : (
          <table>
            <thead>
              <tr><th>ID</th><th>姓名</th><th>邮箱</th><th>操作</th></tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.id}</td>
                  <td>{user.name}</td>
                  <td>{user.email || '—'}</td>
                  <td className="ops">
                    <button className="small" onClick={() => edit(user)}>编辑</button>
                    <button className="small danger" onClick={() => remove(user.id)}>删除</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  )
}
