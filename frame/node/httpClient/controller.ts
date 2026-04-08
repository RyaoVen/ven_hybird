import {HttpClient, HttpHandler, HttpServer} from './httpClient';
import {HTTPClientConfig, HttpServerConfig} from "../config";
import {request} from "./type";



export class httpController {
    private httpClient: HttpClient = new HttpClient(
        HTTPClientConfig.responseURL,
        {
            headers: HTTPClientConfig.headers,
            timeout: HTTPClientConfig.timeout
        }
    )

    private httpHandler: HttpHandler = new HttpHandler();


    //处理请求
    async requestDeal() {
        //注册路由
        this.httpHandler.post('/',(ctx)=>{

        })

        //创建服务
        const sever = new HttpServer(this.httpHandler,HttpServerConfig)
        await sever.start()
    }



    //发起请求
    public requestPost(){

    }
}