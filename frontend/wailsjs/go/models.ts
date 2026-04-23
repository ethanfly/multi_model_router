export namespace main {
	
	export class ChatMessage {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class ChatRequest {
	    messages: ChatMessage[];
	    mode: string;
	    modelId: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], ChatMessage);
	        this.mode = source["mode"];
	        this.modelId = source["modelId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatResponse {
	    modelId: string;
	    modelName: string;
	    provider: string;
	    complexity: string;
	    routeMode: string;
	    status: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modelId = source["modelId"];
	        this.modelName = source["modelName"];
	        this.provider = source["provider"];
	        this.complexity = source["complexity"];
	        this.routeMode = source["routeMode"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class ModelJSON {
	    id: string;
	    name: string;
	    provider: string;
	    baseUrl: string;
	    apiKey: string;
	    modelId: string;
	    reasoning: number;
	    coding: number;
	    creativity: number;
	    speed: number;
	    costEfficiency: number;
	    maxRpm: number;
	    maxTpm: number;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelJSON(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.modelId = source["modelId"];
	        this.reasoning = source["reasoning"];
	        this.coding = source["coding"];
	        this.creativity = source["creativity"];
	        this.speed = source["speed"];
	        this.costEfficiency = source["costEfficiency"];
	        this.maxRpm = source["maxRpm"];
	        this.maxTpm = source["maxTpm"];
	        this.isActive = source["isActive"];
	    }
	}

}

