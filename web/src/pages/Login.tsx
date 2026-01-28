import React from 'react'
import { Button, Card, Form, Input, Typography } from 'antd'

export const LoginPage: React.FC = () => {
  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24
      }}
    >
      <Card style={{ width: 360, borderRadius: 16 }}>
        <Typography.Title level={3} style={{ marginTop: 0 }}>
          Sign in
        </Typography.Title>
        <Form layout="vertical">
          <Form.Item label="Email">
            <Input placeholder="you@company.com" />
          </Form.Item>
          <Form.Item label="Password">
            <Input.Password placeholder="••••••••" />
          </Form.Item>
          <Button type="primary" block>
            Continue
          </Button>
        </Form>
      </Card>
    </div>
  )
}
