import { useCallback, useEffect, useState } from 'react'
import { api } from './api'

const emptyForm = {
  title: '', applicant: '', reason: '', owner: '', owner_manager: '',
}

const STATUS = {
  pending: { label: '审批中', className: 'pending' },
  approved: { label: '已通过', className: 'approved' },
  returned: { label: '已驳回', className: 'rejected' },
}

function formatTime(value) {
  if (!value || value.startsWith('0001-')) return '—'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString('zh-CN', { hour12: false })
}

function StepFlow({ item }) {
  return (
    <div className="flow">
      {item.steps?.map((step, idx) => (
        <div key={idx} className={`flow-step ${step.status}`}>
          <span className="flow-dot">{step.status === 'approved' ? '✓' : step.status === 'rejected' ? '×' : idx + 1}</span>
          <span className="flow-info">
            <strong>{step.name}</strong>
            <small>{step.approver}</small>
            {step.remark && <small className="flow-remark">{step.remark}</small>}
          </span>
        </div>
      ))}
    </div>
  )
}

export default function App() {
  const [items, setItems] = useState([])
  const [filter, setFilter] = useState('')
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setItems(await api.list(filter))
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [filter])

  useEffect(() => {
    api.health()
      .then(() => setConnected(true))
      .catch(() => setConnected(false))
  }, [])

  useEffect(() => { load() }, [load])

  const submit = async (e) => {
    e.preventDefault()
    if (!form.title.trim() || !form.applicant.trim() || !form.owner.trim() || !form.owner_manager.trim()) {
      setError('标题、申请人、需求owner、需求owner上级都不能为空')
      return
    }
    setSaving(true)
    setError('')
    try {
      if (editingId === null) await api.create(form)
      else await api.patch(editingId, form)
      setForm(emptyForm)
      setEditingId(null)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const edit = (item) => {
    setEditingId(item.id)
    setForm({
      title: item.title, applicant: item.applicant, reason: item.reason,
      owner: item.owner, owner_manager: item.owner_manager,
    })
    setError('')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const cancelEdit = () => {
    setEditingId(null)
    setForm(emptyForm)
    setError('')
  }

  const decide = async (id, action) => {
    const label = action === 'approve' ? '通过' : '驳回'
    const remark = window.prompt(`${label}审批意见（可留空）：`) ?? ''
    setError('')
    try {
      if (action === 'approve') await api.approve(id, remark)
      else await api.reject(id, remark)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const resubmit = async (item) => {
    setError('')
    try {
      await api.resubmit(item.id)
      setEditingId(null)
      setForm(emptyForm)
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  const remove = async (id) => {
    if (!window.confirm(`确定删除审批单 #${id} 吗？`)) return
    setError('')
    try {
      await api.remove(id)
      if (editingId === id) cancelEdit()
      await load()
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <div className="page">
      <header className="header">
        <div>
          <h1>审批管理</h1>
          <p>两级审批：需求owner → 需求owner上级（Go REST API）</p>
        </div>
        <span className={`status ${connected === true ? 'ok' : connected === false ? 'bad' : ''}`}>
          {connected === null ? '检测中…' : connected ? '● 服务正常' : '● 服务不可用'}
        </span>
      </header>

      {error && <div className="alert">{error}</div>}

      <section className="card">
        <h2>{editingId === null ? '新建申请' : `修改被驳回的申请 #${editingId}`}</h2>
        <form onSubmit={submit}>
          <div className="row">
            <label>
              标题
              <input
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
                placeholder="例如：采购办公电脑"
              />
            </label>
            <label>
              申请人
              <input
                value={form.applicant}
                onChange={(e) => setForm({ ...form, applicant: e.target.value })}
                placeholder="例如：张三"
              />
            </label>
          </div>
          <div className="row three">
            <label>
              需求owner（第一节点审批人）
              <input
                value={form.owner}
                onChange={(e) => setForm({ ...form, owner: e.target.value })}
                placeholder="例如：李四"
              />
            </label>
            <label>
              需求owner上级（第二节点审批人）
              <input
                value={form.owner_manager}
                onChange={(e) => setForm({ ...form, owner_manager: e.target.value })}
                placeholder="例如：王五"
              />
            </label>
          </div>
          <label className="reason">
            申请事由
            <textarea
              value={form.reason}
              onChange={(e) => setForm({ ...form, reason: e.target.value })}
              placeholder="简要说明申请原因"
              rows={2}
            />
          </label>
          <div className="actions">
            <button type="submit" disabled={saving}>
              {saving ? '保存中…' : editingId === null ? '提交申请' : '保存修改'}
            </button>
            {editingId !== null && (
              <button type="button" className="secondary" onClick={cancelEdit}>取消</button>
            )}
            <button type="button" className="ghost" onClick={load} disabled={loading}>刷新列表</button>
          </div>
        </form>
      </section>

      <section className="card">
        <div className="list-head">
          <h2>审批单 <span className="count">{items.length}</span></h2>
          <div className="filters">
            {[
              { value: '', label: '全部' },
              { value: 'pending', label: '审批中' },
              { value: 'approved', label: '已通过' },
              { value: 'returned', label: '已驳回' },
            ].map((f) => (
              <button
                key={f.value}
                type="button"
                className={`filter ${filter === f.value ? 'active' : ''}`}
                onClick={() => setFilter(f.value)}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {loading ? (
          <p className="muted">加载中…</p>
        ) : items.length === 0 ? (
          <p className="muted">暂无审批单，先在上方新建一个吧。</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th><th>标题</th><th>申请人</th><th>状态</th>
                  <th>审批节点</th><th>提交时间</th><th>操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => {
                  const st = STATUS[item.status] || { label: item.status, className: '' }
                  return (
                    <tr key={item.id}>
                      <td>{item.id}</td>
                      <td className="title-cell">
                        {item.title}
                        {item.status === 'returned' && item.last_reject_reason && (
                          <div className="reject-reason">驳回原因：{item.last_reject_reason}</div>
                        )}
                      </td>
                      <td>{item.applicant}</td>
                      <td><span className={`badge ${st.className}`}>{st.label}</span></td>
                      <td><StepFlow item={item} /></td>
                      <td className="time-cell">{formatTime(item.created_at)}</td>
                      <td className="ops">
                        {item.status === 'pending' && (
                          <>
                            <button className="small approve" onClick={() => decide(item.id, 'approve')}>通过</button>
                            <button className="small reject" onClick={() => decide(item.id, 'reject')}>驳回</button>
                          </>
                        )}
                        {item.status === 'returned' && (
                          <>
                            <button className="small" onClick={() => edit(item)}>修改</button>
                            <button className="small resubmit" onClick={() => resubmit(item)}>重新提交</button>
                          </>
                        )}
                        <button className="small danger" onClick={() => remove(item.id)}>删除</button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}
