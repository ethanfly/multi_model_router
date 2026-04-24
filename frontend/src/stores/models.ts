import { ref } from 'vue'
import { defineStore } from 'pinia'
import {
  GetModels,
  SaveModel,
  DeleteModel,
  ExportModels,
  ImportModels,
} from '../../wailsjs/go/main/App'
import { core } from '../../wailsjs/go/models'

type Model = core.ModelJSON

export const useModelsStore = defineStore('models', () => {
  const models = ref<Model[]>([])
  const loading = ref(false)

  async function fetchModels() {
    loading.value = true
    try {
      models.value = (await GetModels()) || []
    } catch (err) {
      console.error('Failed to fetch models:', err)
    } finally {
      loading.value = false
    }
  }

  async function save(model: Model) {
    try {
      await SaveModel(model)
      await fetchModels()
    } catch (err) {
      console.error('Failed to save model:', err)
      throw err
    }
  }

  async function remove(id: string) {
    try {
      await DeleteModel(id)
      await fetchModels()
    } catch (err) {
      console.error('Failed to delete model:', err)
      throw err
    }
  }

  async function exportModels(password: string): Promise<string> {
    return await ExportModels(password)
  }

  async function importModels(jsonData: string, password: string): Promise<string> {
    const result = await ImportModels(jsonData, password)
    await fetchModels()
    return result
  }

  return { models, loading, fetchModels, save, remove, exportModels, importModels }
})

export type { Model }
