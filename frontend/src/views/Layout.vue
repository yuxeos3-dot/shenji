<template>
  <el-container style="min-height:100vh">
    <el-aside width="220px" style="background:#304156">
      <div style="padding:20px;text-align:center;color:#fff;font-size:16px;font-weight:bold">SEO巡站系统</div>
      <el-menu :default-active="$route.path" router background-color="#304156" text-color="#bfcbd9" active-text-color="#409EFF">
        <el-menu-item index="/dashboard"><el-icon><DataAnalysis /></el-icon><span>仪表盘</span></el-menu-item>
        <el-menu-item index="/sites"><el-icon><Monitor /></el-icon><span>站点管理</span></el-menu-item>
        <el-menu-item index="/reports"><el-icon><Document /></el-icon><span>每日汇报</span></el-menu-item>
        <el-menu-item v-if="userStore.isAdmin()" index="/users"><el-icon><UserFilled /></el-icon><span>用户管理</span></el-menu-item>
        <el-menu-item v-if="userStore.isAdmin()" index="/settings"><el-icon><Setting /></el-icon><span>系统设置</span></el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header style="display:flex;align-items:center;justify-content:flex-end;background:#fff;box-shadow:0 1px 4px rgba(0,21,41,.08)">
        <span style="margin-right:16px">{{ userStore.user?.name }}</span>
        <el-dropdown @command="handleCommand">
          <el-button type="primary" text>
            <el-icon><UserFilled /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="password">修改密码</el-dropdown-item>
              <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main style="background:#f0f2f5;padding:20px">
        <router-view />
      </el-main>
    </el-container>
  </el-container>

  <el-dialog v-model="showPasswordDialog" title="修改密码" width="400px">
    <el-form :model="pwdForm">
      <el-form-item label="旧密码"><el-input v-model="pwdForm.old_password" type="password" show-password /></el-form-item>
      <el-form-item label="新密码"><el-input v-model="pwdForm.new_password" type="password" show-password /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showPasswordDialog=false">取消</el-button>
      <el-button type="primary" @click="changePassword">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const router = useRouter()
const userStore = useUserStore()
const showPasswordDialog = ref(false)
const pwdForm = ref({ old_password: '', new_password: '' })

onMounted(() => { userStore.fetchUser() })

function handleCommand(cmd) {
  if (cmd === 'logout') {
    userStore.logout()
    router.push('/login')
  } else if (cmd === 'password') {
    showPasswordDialog.value = true
  }
}

async function changePassword() {
  try {
    await apiClient.changePassword(pwdForm.value)
    ElMessage.success('密码修改成功')
    showPasswordDialog.value = false
    pwdForm.value = { old_password: '', new_password: '' }
  } catch {}
}
</script>
