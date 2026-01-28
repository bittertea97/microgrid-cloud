import React, { useMemo } from 'react'
import { Layout, Menu } from 'antd'
import {
  BarChartOutlined,
  AlertOutlined,
  DatabaseOutlined,
  ExportOutlined
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { TopBar } from './TopBar'

const { Sider, Content } = Layout

const menuItems = [
  {
    key: '/app/analytics/stats',
    icon: <BarChartOutlined />,
    label: 'Stats'
  },
  {
    key: '/app/analytics/settlements',
    icon: <DatabaseOutlined />,
    label: 'Settlements'
  },
  {
    key: '/app/analytics/exports',
    icon: <ExportOutlined />,
    label: 'Exports'
  },
  {
    key: '/app/alarms',
    icon: <AlertOutlined />,
    label: 'Alarms'
  }
]

export const AppLayout: React.FC = () => {
  const location = useLocation()
  const navigate = useNavigate()

  const selectedKeys = useMemo(() => {
    const match = menuItems.find((item) => location.pathname.startsWith(item.key))
    return match ? [match.key] : []
  }, [location.pathname])

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        width={220}
        style={{
          background: '#0b1f33',
          color: '#ffffff'
        }}
      >
        <div
          style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            padding: '0 20px',
            color: '#ffffff',
            fontWeight: 600,
            letterSpacing: 0.5
          }}
        >
          Microgrid Cloud
        </div>
        <Menu
          theme="dark"
          mode="inline"
          items={menuItems}
          selectedKeys={selectedKeys}
          onClick={(info) => navigate(info.key)}
        />
      </Sider>
      <Layout>
        <TopBar />
        <Content className="page-shell">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
