import { z } from 'zod'

export const loginSchema = z.object({
  email: z.string().min(1, '请填写邮箱').email('邮箱格式不正确'),
  password: z.string().min(1, '请填写密码'),
})

export const createUserSchema = z.object({
  name: z.string().min(1, '请填写姓名').max(50, '姓名不超过50字'),
  email: z.string().min(1, '请填写邮箱').email('邮箱格式不正确'),
  password: z.string()
    .min(12, '密码至少12位')
    .regex(/[A-Z]/, '需包含大写字母')
    .regex(/[a-z]/, '需包含小写字母')
    .regex(/[0-9]/, '需包含数字'),
  role: z.enum(['admin', 'member']).default('member'),
})

export const createSiteSchema = z.object({
  domain: z.string().min(1, '请填写域名').url('请输入完整URL'),
  name: z.string().min(1, '请填写名称').max(100, '名称不超过100字'),
})

export const changePasswordSchema = z.object({
  oldPassword: z.string().min(1, '请填写旧密码'),
  newPassword: z.string()
    .min(12, '密码至少12位')
    .regex(/[A-Z]/, '需包含大写字母')
    .regex(/[a-z]/, '需包含小写字母')
    .regex(/[0-9]/, '需包含数字'),
  confirmPassword: z.string().min(1, '请确认新密码'),
}).refine((data) => data.newPassword === data.confirmPassword, {
  message: '两次密码不一致',
  path: ['confirmPassword'],
})
